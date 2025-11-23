import { useState, useEffect, useRef } from 'react';
import { useUI } from '../context/UIContext';
import DebugLogViewer from '../components/shared/DebugLogViewer';

export const useDebugLogs = () => {
    const [debugLogs, setDebugLogs] = useState([]);
    const isListenerRegistered = useRef(false);
    const { openTab, bottomTabs, setBottomTabs } = useUI(); // We need setBottomTabs to update content

    useEffect(() => {
        if (window.runtime && !isListenerRegistered.current) {
            console.log("Registering debug-log listener");
            window.runtime.EventsOn("debug-log", (msg) => {
                console.log("Backend Debug Log Received:", msg);
                setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] ${msg}`]);
            });
            isListenerRegistered.current = true;
        }

        return () => {
            if (window.runtime && isListenerRegistered.current) {
                window.runtime.EventsOff("debug-log");
                isListenerRegistered.current = false;
            }
        };
    }, []);

    // Sync debug logs to the tab content whenever they change
    useEffect(() => {
        setBottomTabs(prev => prev.map(tab => {
            if (tab.id === 'debug-logs') {
                return {
                    ...tab,
                    content: (
                        <DebugLogViewer
                            logs={debugLogs}
                            onTestEmit={(type) => {
                                if (type === 'ui') {
                                    setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Test Log(UI Only)`]);
                                } else {
                                    window.go.main.App.TestEmit();
                                }
                            }}
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

        if (!existingTab) {
            openTab({
                id: debugTabId,
                title: 'Debug Logs',
                content: (
                    <DebugLogViewer
                        logs={debugLogs}
                        onTestEmit={(type) => {
                            if (type === 'ui') {
                                setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Test Log(UI Only)`]);
                            } else {
                                window.go.main.App.TestEmit();
                            }
                        }}
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
