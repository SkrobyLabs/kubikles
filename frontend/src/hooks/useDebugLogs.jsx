import { useState, useEffect, useRef, useCallback } from 'react';
import { useUI } from '../context/UIContext';
import { useDebug } from '../context/DebugContext';
import { useNotification } from '../context/NotificationContext';
import DebugLogViewer from '../components/shared/DebugLogViewer';
import { SaveLogFile } from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

// Global event listener - only one instance to avoid Wails EventsOff issues
let globalEventHandler = null;
const subscribers = new Set();

export const useDebugLogs = () => {
    const [debugLogs, setDebugLogs] = useState([]);
    const { openTab, bottomTabs, setBottomTabs } = useUI();
    const { enableDebugMode } = useDebug();
    const { addNotification } = useNotification();

    // Create a subscriber callback using ref to avoid stale closures
    const subscriberRef = useRef(null);

    subscriberRef.current = useCallback((msg) => {
        setDebugLogs(prev => {
            const newLogs = [...prev, `[${new Date().toLocaleTimeString()}] ${msg}`];
            if (newLogs.length > 1000) {
                return newLogs.slice(newLogs.length - 1000);
            }
            return newLogs;
        });
    }, []);

    // Subscribe to global event system
    useEffect(() => {
        // Create wrapper that calls through ref (avoids stale closure)
        const subscriber = (msg) => {
            if (subscriberRef.current) {
                subscriberRef.current(msg);
            }
        };

        subscribers.add(subscriber);

        // Set up global handler if not already done
        if (!globalEventHandler) {
            console.log("Registering debug-log listener");
            globalEventHandler = (msg) => {
                subscribers.forEach(sub => sub(msg));
            };
            EventsOn("debug-log", globalEventHandler);
        }

        return () => {
            subscribers.delete(subscriber);
            // Only remove global handler when no more subscribers
            if (subscribers.size === 0 && globalEventHandler) {
                EventsOff("debug-log");
                globalEventHandler = null;
            }
        };
    }, []);

    const clearLogs = () => setDebugLogs([]);

    const downloadLogs = async () => {
        try {
            const content = debugLogs.join('\n');
            await SaveLogFile(content);
        } catch (err) {
            console.error("Failed to save logs:", err);
            addNotification({ type: 'error', title: 'Failed to save logs', message: String(err) });
        }
    };

    // Sync debug logs to the tab content whenever they change
    useEffect(() => {
        setBottomTabs(prev => prev.map(tab => {
            if (tab.id === 'debug-logs') {
                return {
                    ...tab,
                    content: (
                        <DebugLogViewer
                            logs={debugLogs}
                            onClear={clearLogs}
                            onDownload={downloadLogs}
                        />
                    )
                };
            }
            return tab;
        }));
    }, [debugLogs, setBottomTabs]);

    const toggleDebug = () => {
        const debugTabId = 'debug-logs';
        const existingTab = bottomTabs.find(t => t.id === debugTabId);

        // Enable debug mode globally when opening debug logs
        enableDebugMode();

        if (!existingTab) {
            openTab({
                id: debugTabId,
                title: 'Debug Logs',
                context: null, // Debug logs are context-independent
                content: (
                    <DebugLogViewer
                        logs={debugLogs}
                        onClear={clearLogs}
                        onDownload={downloadLogs}
                    />
                )
            });
        } else {
            // If it exists, openTab will just set it as active
            openTab(existingTab);
        }
    };

    return { toggleDebug };
};
