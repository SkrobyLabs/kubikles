import React, { useState, useEffect, useRef } from 'react';
import Terminal from '../../../components/shared/Terminal';
import {
    CreateNodeDebugPod,
    OpenTerminalWithCommand,
    DeletePod,
    ListPods
} from '../../../../wailsjs/go/main/App';
import Logger from '../../../utils/Logger';

// Helper to wait for a pod to be running
const waitForPodRunning = async (namespace, podName, timeoutMs = 60000, pollIntervalMs = 1000) => {
    const startTime = Date.now();

    while (Date.now() - startTime < timeoutMs) {
        try {
            const pods = await ListPods(namespace);
            const pod = pods.find(p => p.metadata.name === podName);

            if (pod) {
                const phase = pod.status?.phase;
                if (phase === 'Running') {
                    return true;
                }
                if (phase === 'Failed' || phase === 'Succeeded') {
                    throw new Error(`Pod entered ${phase} state`);
                }
            }
        } catch (err) {
            Logger.warn("Error checking pod status", err);
        }

        await new Promise(resolve => setTimeout(resolve, pollIntervalMs));
    }

    throw new Error('Timeout waiting for pod to be running');
};

const NodeShellTab = ({ nodeName, context }) => {
    const [state, setState] = useState('loading'); // loading, connected, error
    const [terminalUrl, setTerminalUrl] = useState(null);
    const [error, setError] = useState(null);
    const debugPodInfoRef = useRef(null);
    const initStartedRef = useRef(false);

    useEffect(() => {
        // Prevent double initialization in React strict mode
        if (initStartedRef.current) return;
        initStartedRef.current = true;

        const initialize = async () => {
            try {
                Logger.info("Creating debug pod for node shell", { node: nodeName });
                const debugPodInfo = await CreateNodeDebugPod(nodeName);
                debugPodInfoRef.current = debugPodInfo;
                Logger.info("Debug pod created", { podName: debugPodInfo.podName, namespace: debugPodInfo.namespace });

                Logger.info("Waiting for debug pod to be running...");
                await waitForPodRunning(debugPodInfo.namespace, debugPodInfo.podName);
                Logger.info("Debug pod is running");

                const url = await OpenTerminalWithCommand(context, debugPodInfo.namespace, debugPodInfo.podName, "shell", "nsenter");
                setTerminalUrl(url);
                setState('connected');
                Logger.info("Shell opened successfully", { node: nodeName });
            } catch (err) {
                Logger.error("Failed to open shell on node", err);
                setError(String(err));
                setState('error');

                // Cleanup debug pod if it was created
                if (debugPodInfoRef.current) {
                    try {
                        await DeletePod(context, debugPodInfoRef.current.namespace, debugPodInfoRef.current.podName);
                    } catch (cleanupErr) {
                        Logger.warn("Failed to cleanup debug pod after error", cleanupErr);
                    }
                }
            }
        };

        initialize();
    }, [nodeName, context]);

    const handleTerminalClose = async () => {
        if (debugPodInfoRef.current) {
            try {
                Logger.info("Cleaning up debug pod", { podName: debugPodInfoRef.current.podName });
                await DeletePod(context, debugPodInfoRef.current.namespace, debugPodInfoRef.current.podName);
                Logger.info("Debug pod deleted successfully");
            } catch (err) {
                Logger.warn("Failed to cleanup debug pod", err);
            }
        }
    };

    if (state === 'loading') {
        return (
            <div className="h-full w-full bg-[#1e1e1e] flex items-center justify-center">
                <div className="text-center">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4"></div>
                    <p className="text-gray-400">Scheduling debug pod on {nodeName}...</p>
                </div>
            </div>
        );
    }

    if (state === 'error') {
        return (
            <div className="h-full w-full bg-[#1e1e1e] flex items-center justify-center">
                <div className="text-center">
                    <p className="text-red-400 mb-2">Failed to open shell</p>
                    <p className="text-gray-500 text-sm">{error}</p>
                </div>
            </div>
        );
    }

    return <Terminal url={terminalUrl} onClose={handleTerminalClose} />;
};

export default NodeShellTab;
