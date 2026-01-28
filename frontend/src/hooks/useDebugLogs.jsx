import { useState, useEffect, useRef } from 'react';
import { useUI } from '../context/UIContext';
import { useDebug } from '../context/DebugContext';
import { useNotification } from '../context/NotificationContext';
import DebugLogViewer from '../components/shared/DebugLogViewer';
import { SaveLogFile } from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

export const useDebugLogs = () => {
    const [debugLogs, setDebugLogs] = useState([]);
    const isListenerRegistered = useRef(false);
    const { openTab, bottomTabs, setBottomTabs } = useUI(); // We need setBottomTabs to update content
    const { enableDebugMode } = useDebug();
    const { addNotification } = useNotification();

    useEffect(() => {
        if (!isListenerRegistered.current) {
            console.log("Registering debug-log listener");
            EventsOn("debug-log", (msg) => {
                setDebugLogs(prev => {
                    const newLogs = [...prev, `[${new Date().toLocaleTimeString()}] ${msg}`];
                    if (newLogs.length > 1000) {
                        return newLogs.slice(newLogs.length - 1000);
                    }
                    return newLogs;
                });
            });
            isListenerRegistered.current = true;
        }

        return () => {
            if (isListenerRegistered.current) {
                EventsOff("debug-log");
                isListenerRegistered.current = false;
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
