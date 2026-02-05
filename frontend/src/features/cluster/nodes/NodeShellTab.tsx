import React, { useState, useRef } from 'react';
import { LazyTerminal as Terminal } from '../../../components/lazy';
import {
    CreateNodeDebugPod,
    DeletePod,
    ListPods
} from '../../../../wailsjs/go/main/App';
import { useConfig } from '../../../context';
import Logger from '../../../utils/Logger';

// Preset debug images
const PRESET_IMAGES = [
    { name: 'BusyBox', image: 'busybox:latest', description: 'Minimal Unix tools' },
    { name: 'Alpine', image: 'alpine:latest', description: 'Lightweight Linux' },
    { name: 'Netshoot', image: 'nicolaka/netshoot:latest', description: 'Network debugging tools' },
];

// Helper to wait for a pod to be running
const waitForPodRunning = async (namespace, podName, timeoutMs = 60000, pollIntervalMs = 1000) => {
    const startTime = Date.now();

    while (Date.now() - startTime < timeoutMs) {
        try {
            const pods = await ListPods('', namespace);
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
    const { getConfig } = useConfig();
    const defaultImage = getConfig('kubernetes.nodeDebugImage') || 'alpine:latest';

    const [state, setState] = useState('selecting'); // selecting, loading, connected, error
    const [selectedImage, setSelectedImage] = useState(defaultImage);
    const [customImage, setCustomImage] = useState('');
    const [debugPodInfo, setDebugPodInfo] = useState(null);
    const [error, setError] = useState(null);
    const debugPodInfoRef = useRef(null);

    const handleConnect = async () => {
        const imageToUse = customImage.trim() || selectedImage;
        setState('loading');

        try {
            Logger.info("Creating debug pod for node shell", { node: nodeName, image: imageToUse });
            const podInfo = await CreateNodeDebugPod(nodeName, imageToUse);
            debugPodInfoRef.current = podInfo;
            setDebugPodInfo(podInfo);
            Logger.info("Debug pod created", { podName: podInfo.podName, namespace: podInfo.namespace });

            Logger.info("Waiting for debug pod to be running...");
            await waitForPodRunning(podInfo.namespace, podInfo.podName);
            Logger.info("Debug pod is running");

            setState('connected');
            Logger.info("Shell opened successfully", { node: nodeName });
        } catch (err) {
            Logger.error("Failed to open shell on node", err);
            setError(String(err));
            setState('error');

            // Cleanup debug pod if it was created
            if (debugPodInfoRef.current) {
                try {
                    await DeletePod(debugPodInfoRef.current.namespace, debugPodInfoRef.current.podName);
                } catch (cleanupErr) {
                    Logger.warn("Failed to cleanup debug pod after error", cleanupErr);
                }
            }
        }
    };

    const handleTerminalClose = async () => {
        if (debugPodInfoRef.current) {
            try {
                Logger.info("Cleaning up debug pod", { podName: debugPodInfoRef.current.podName });
                await DeletePod(debugPodInfoRef.current.namespace, debugPodInfoRef.current.podName);
                Logger.info("Debug pod deleted successfully");
            } catch (err) {
                Logger.warn("Failed to cleanup debug pod", err);
            }
        }
    };

    const handlePresetClick = (image) => {
        setSelectedImage(image);
        setCustomImage('');
    };

    if (state === 'selecting') {
        return (
            <div className="h-full w-full bg-background overflow-auto">
                <div className="max-w-md w-full p-6 mx-auto">
                    <h3 className="text-lg font-medium text-foreground mb-4">Select Debug Image</h3>
                    <p className="text-sm text-muted-foreground mb-4">
                        Choose a container image for the debug pod on <span className="font-medium text-foreground">{nodeName}</span>
                    </p>

                    {/* Preset images */}
                    <div className="flex flex-wrap gap-2 mb-4">
                        {PRESET_IMAGES.map((preset) => (
                            <button
                                key={preset.image}
                                onClick={() => handlePresetClick(preset.image)}
                                className={`px-3 py-2 rounded-md text-sm transition-colors ${
                                    selectedImage === preset.image && !customImage
                                        ? 'bg-blue-600 text-white'
                                        : 'bg-muted hover:bg-muted/80 text-foreground'
                                }`}
                                title={preset.description}
                            >
                                {preset.name}
                            </button>
                        ))}
                    </div>

                    {/* Custom image input */}
                    <div className="mb-4">
                        <label className="block text-sm text-muted-foreground mb-2">
                            Or enter custom image:
                        </label>
                        <input
                            type="text"
                            value={customImage}
                            onChange={(e) => setCustomImage(e.target.value)}
                            placeholder="e.g., ubuntu:22.04"
                            className="w-full px-3 py-2 bg-background border border-border rounded-md text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
                        />
                    </div>

                    {/* Selected image display */}
                    <div className="mb-4 p-2 bg-muted rounded-md">
                        <span className="text-sm text-muted-foreground">Image: </span>
                        <span className="text-sm font-mono text-foreground">
                            {customImage.trim() || selectedImage}
                        </span>
                    </div>

                    {/* Connect button */}
                    <button
                        onClick={handleConnect}
                        className="w-full px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-md transition-colors font-medium"
                    >
                        Connect
                    </button>
                </div>
            </div>
        );
    }

    if (state === 'loading') {
        return (
            <div className="h-full w-full bg-background flex items-center justify-center">
                <div className="text-center">
                    <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-blue-500 mx-auto mb-4"></div>
                    <p className="text-gray-400">Scheduling debug pod on {nodeName}...</p>
                    <p className="text-gray-500 text-sm mt-1">Image: {customImage.trim() || selectedImage}</p>
                </div>
            </div>
        );
    }

    if (state === 'error') {
        return (
            <div className="h-full w-full bg-background flex items-center justify-center">
                <div className="text-center select-text">
                    <p className="text-red-400 mb-2">Failed to open shell</p>
                    <p className="text-gray-500 text-sm">{error}</p>
                </div>
            </div>
        );
    }

    return (
        <Terminal
            namespace={debugPodInfo?.namespace}
            pod={debugPodInfo?.podName}
            container="shell"
            context={context}
            command="nsenter"
            onClose={handleTerminalClose}
        />
    );
};

export default NodeShellTab;
