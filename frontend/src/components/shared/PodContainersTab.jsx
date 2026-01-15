import React, { useState, useMemo, useCallback } from 'react';
import { ChevronDownIcon, SignalIcon, ClipboardDocumentIcon, CheckIcon, PlayIcon, StopIcon, TrashIcon, ArrowTopRightOnSquareIcon, CommandLineIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { usePortForwards } from '../../hooks/usePortForwards';
import { useUI } from '../../context/UIContext';
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime';
import { OpenTerminal } from '../../../wailsjs/go/main/App';
import PodPortForwardDialog from './PodPortForwardDialog';
import Terminal from './Terminal';

// Copy button component
const CopyButton = ({ value }) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = async (e) => {
        e.stopPropagation();
        if (!value) return;
        try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <button
            onClick={handleCopy}
            className="p-1 text-gray-500 hover:text-gray-300 transition-colors"
            title={copied ? 'Copied!' : 'Copy to clipboard'}
        >
            {copied ? (
                <CheckIcon className="w-4 h-4 text-green-400" />
            ) : (
                <ClipboardDocumentIcon className="w-4 h-4" />
            )}
        </button>
    );
};

// Volume mount label with copy functionality
const VolumeMountLabel = ({ mount }) => {
    const [copied, setCopied] = useState(false);
    const copyValue = `${mount.name}: ${mount.mountPath}`;

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(copyValue);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <button
            onClick={handleCopy}
            className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded border transition-colors cursor-pointer ${
                copied
                    ? 'bg-green-500/20 text-green-400 border-green-500/30'
                    : 'bg-gray-500/10 hover:bg-gray-500/20 text-gray-300 border-gray-500/30'
            }`}
            title={copied ? 'Copied!' : `Click to copy: ${copyValue}`}
        >
            {copied ? (
                <>
                    <CheckIcon className="w-3 h-3" />
                    Copied
                </>
            ) : (
                <>
                    <span className="text-gray-400">{mount.name}:</span>
                    <code className="font-mono">{mount.mountPath}</code>
                    {mount.readOnly ? (
                        <span className="text-yellow-400 text-[10px]">RO</span>
                    ) : (
                        <span className="text-green-400 text-[10px]">RW</span>
                    )}
                </>
            )}
        </button>
    );
};

// Helper to extract image SHA from image ID
const extractImageSha = (imageId, full = false) => {
    if (!imageId) return 'N/A';
    // Format: docker://sha256:abc123... or docker-pullable://image@sha256:abc123
    const sha256Match = imageId.match(/sha256:([a-f0-9]+)/);
    if (sha256Match) {
        const hash = sha256Match[1];
        if (full) return hash;
        // Show first 6 and last 6 characters with ellipsis
        return hash.length > 12 ? `${hash.substring(0, 6)}...${hash.substring(hash.length - 6)}` : hash;
    }
    return imageId.split('/').pop()?.substring(0, 40) || 'N/A';
};

// Helper to format resource quantities
const formatResources = (resources) => {
    if (!resources || Object.keys(resources).length === 0) {
        return 'Not set';
    }
    const parts = [];
    if (resources.cpu) parts.push(`CPU: ${resources.cpu}`);
    if (resources.memory) parts.push(`Mem: ${resources.memory}`);
    if (resources['ephemeral-storage']) parts.push(`Storage: ${resources['ephemeral-storage']}`);
    return parts.length > 0 ? parts.join(', ') : 'Not set';
};

// Status badge component
const StatusBadge = ({ state }) => {
    if (!state) return <span className="text-gray-500">Unknown</span>;

    if (state.running) {
        return (
            <span className="px-2 py-0.5 text-xs rounded bg-green-500/20 text-green-400 border border-green-500/30">
                Running
            </span>
        );
    }
    if (state.waiting) {
        return (
            <span className="px-2 py-0.5 text-xs rounded bg-yellow-500/20 text-yellow-400 border border-yellow-500/30" title={state.waiting.message}>
                Waiting: {state.waiting.reason || 'Unknown'}
            </span>
        );
    }
    if (state.terminated) {
        const color = state.terminated.exitCode === 0 ? 'green' : 'red';
        return (
            <span className={`px-2 py-0.5 text-xs rounded bg-${color}-500/20 text-${color}-400 border border-${color}-500/30`} title={state.terminated.message}>
                Terminated: {state.terminated.reason || `Exit ${state.terminated.exitCode}`}
            </span>
        );
    }
    return <span className="text-gray-500">Unknown</span>;
};

// Detail row component
const DetailRow = ({ label, value, children }) => (
    <div className="flex py-2 border-b border-border/50">
        <div className="w-32 text-xs font-medium text-gray-500 uppercase tracking-wider shrink-0">
            {label}
        </div>
        <div className="flex-1 text-sm text-gray-200">
            {children || value || <span className="text-gray-500">N/A</span>}
        </div>
    </div>
);

export default function PodContainersTab({ pod, isStale }) {
    const { currentContext } = useK8s();
    const { configs, activeForwards, startForward, stopForward, deleteConfig } = usePortForwards(currentContext, true);
    const { openModal, closeModal, openTab } = useUI();

    // Find port forward config for a specific port
    const getPortForwardConfig = useCallback((containerPort) => {
        return configs.find(c =>
            c.resourceType === 'pod' &&
            c.resourceName === pod.metadata?.name &&
            c.namespace === pod.metadata?.namespace &&
            c.remotePort === containerPort
        );
    }, [configs, pod.metadata?.name, pod.metadata?.namespace]);

    // Get status for a config ID from activeForwards
    const getConfigStatus = useCallback((configId) => {
        const af = activeForwards.find(af => af.config?.id === configId);
        return af?.status || 'stopped';
    }, [activeForwards]);

    // Get styling for a port based on port forward status
    const getPortStyle = useCallback((containerPort) => {
        const config = getPortForwardConfig(containerPort);
        if (!config) {
            // No rule - gray
            return {
                className: 'bg-gray-500/10 hover:bg-gray-500/20 text-gray-400 border-gray-500/30',
                title: 'Click to create port forward'
            };
        }
        const status = getConfigStatus(config.id);
        switch (status) {
            case 'running':
                // Running - green
                return {
                    className: 'bg-green-500/10 hover:bg-green-500/20 text-green-400 border-green-500/30',
                    title: 'Port forward running - click to manage'
                };
            case 'error':
                // Error - red
                return {
                    className: 'bg-red-500/10 hover:bg-red-500/20 text-red-400 border-red-500/30',
                    title: 'Port forward error - click to manage'
                };
            default:
                // Has rule but stopped - blue
                return {
                    className: 'bg-primary/10 hover:bg-primary/20 text-primary border-primary/30',
                    title: 'Port forward configured - click to manage'
                };
        }
    }, [getPortForwardConfig, getConfigStatus]);

    // Handle start/stop toggle for a port forward
    const handleToggleForward = useCallback(async (e, config) => {
        e.stopPropagation();
        const status = getConfigStatus(config.id);
        try {
            if (status === 'running') {
                await stopForward(config.id);
            } else {
                await startForward(config.id);
            }
        } catch (err) {
            console.error('Failed to toggle port forward:', err);
        }
    }, [getConfigStatus, startForward, stopForward]);

    // Handle delete for a port forward
    const handleDeleteForward = useCallback((e, config) => {
        e.stopPropagation();
        openModal({
            title: 'Delete Port Forward',
            content: `Are you sure you want to delete the port forward for port ${config.remotePort}?`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    const status = getConfigStatus(config.id);
                    if (status === 'running') {
                        await stopForward(config.id);
                    }
                    await deleteConfig(config.id);
                    closeModal();
                } catch (err) {
                    console.error('Failed to delete port forward:', err);
                }
            }
        });
    }, [openModal, closeModal, getConfigStatus, stopForward, deleteConfig]);

    // Handle open in browser
    const handleOpenBrowser = useCallback((e, config) => {
        e.stopPropagation();
        const protocol = config.https ? 'https' : 'http';
        BrowserOpenURL(`${protocol}://localhost:${config.localPort}`);
    }, []);

    // Handle shell into container
    const handleShell = useCallback(async (containerName) => {
        try {
            const url = await OpenTerminal(currentContext, pod.metadata?.namespace, pod.metadata?.name, containerName);
            const tabId = `shell-${pod.metadata?.name}-${containerName}`;
            openTab({
                id: tabId,
                title: `Shell: ${pod.metadata?.name}/${containerName}`,
                keepAlive: true,
                content: <Terminal url={url} />
            });
        } catch (err) {
            console.error('Failed to open shell:', err);
        }
    }, [currentContext, pod.metadata?.namespace, pod.metadata?.name, openTab]);

    // Get containers from spec and match with status
    const containers = useMemo(() => {
        const specContainers = pod.spec?.containers || [];
        const statusContainers = pod.status?.containerStatuses || [];

        return specContainers.map((spec) => {
            const status = statusContainers.find((s) => s.name === spec.name);
            return {
                name: spec.name,
                spec,
                status
            };
        });
    }, [pod]);

    // Include init containers
    const initContainers = useMemo(() => {
        const specContainers = pod.spec?.initContainers || [];
        const statusContainers = pod.status?.initContainerStatuses || [];

        return specContainers.map((spec) => {
            const status = statusContainers.find((s) => s.name === spec.name);
            return {
                name: spec.name,
                spec,
                status,
                isInit: true
            };
        });
    }, [pod]);

    const allContainers = useMemo(() => [...initContainers, ...containers], [initContainers, containers]);

    const [selectedContainer, setSelectedContainer] = useState(containers[0]?.name || initContainers[0]?.name || '');
    const [dropdownOpen, setDropdownOpen] = useState(false);
    const [portForwardDialog, setPortForwardDialog] = useState({ open: false, port: null, existingConfig: null });

    const currentContainer = useMemo(() => {
        return allContainers.find((c) => c.name === selectedContainer) || allContainers[0];
    }, [allContainers, selectedContainer]);

    const handlePortClick = useCallback((port) => {
        const existingConfig = getPortForwardConfig(port.containerPort);
        setPortForwardDialog({
            open: true,
            port: port,
            existingConfig: existingConfig || null
        });
    }, [getPortForwardConfig]);

    const handleClosePortForward = useCallback(() => {
        setPortForwardDialog({ open: false, port: null, existingConfig: null });
    }, []);

    if (allContainers.length === 0) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500">
                No containers found in this pod.
            </div>
        );
    }

    const { spec, status, isInit } = currentContainer || {};
    const ports = spec?.ports || [];
    const requests = spec?.resources?.requests;
    const limits = spec?.resources?.limits;

    return (
        <div className="h-full overflow-auto p-4">
            {/* Container Selector */}
            <div className="mb-6">
                <label className="block text-xs font-medium text-gray-500 uppercase tracking-wider mb-2">
                    Container
                </label>
                <div className="flex items-center gap-2">
                    <div className="relative inline-block">
                        <button
                            onClick={() => setDropdownOpen(!dropdownOpen)}
                            className="flex items-center gap-2 px-3 py-2 bg-surface border border-border rounded-lg text-sm hover:bg-surface-light transition-colors min-w-[200px]"
                        >
                            <span className="flex-1 text-left">
                                {currentContainer?.name}
                                {isInit && <span className="ml-2 text-xs text-yellow-400">(init)</span>}
                            </span>
                            <ChevronDownIcon className={`w-4 h-4 text-gray-400 transition-transform ${dropdownOpen ? 'rotate-180' : ''}`} />
                        </button>
                        {dropdownOpen && (
                            <div className="absolute top-full left-0 mt-1 w-full bg-surface border border-border rounded-lg shadow-lg z-10 py-1 max-h-60 overflow-auto">
                                {allContainers.map((c) => (
                                    <button
                                        key={c.name}
                                        onClick={() => {
                                            setSelectedContainer(c.name);
                                            setDropdownOpen(false);
                                        }}
                                        className={`w-full px-3 py-2 text-sm text-left hover:bg-white/5 ${
                                            c.name === selectedContainer ? 'bg-primary/10 text-primary' : 'text-gray-300'
                                        }`}
                                    >
                                        {c.name}
                                        {c.isInit && <span className="ml-2 text-xs text-yellow-400">(init)</span>}
                                    </button>
                                ))}
                            </div>
                        )}
                    </div>
                    <button
                        onClick={() => handleShell(currentContainer?.name)}
                        className="p-2 text-gray-400 hover:text-white hover:bg-white/10 rounded-lg border border-border transition-colors"
                        title={`Shell into ${currentContainer?.name}`}
                    >
                        <CommandLineIcon className="w-4 h-4" />
                    </button>
                </div>
            </div>

            {/* Container Details */}
            <div className="bg-surface rounded-lg border border-border p-4">
                <DetailRow label="Status">
                    <StatusBadge state={status?.state} />
                    {status?.restartCount > 0 && (
                        <span className="ml-3 text-xs text-gray-500">
                            ({status.restartCount} restart{status.restartCount !== 1 ? 's' : ''})
                        </span>
                    )}
                </DetailRow>

                <DetailRow label="Image">
                    <div className="flex items-center gap-1">
                        <span className="truncate" title={spec?.image}>{spec?.image || <span className="text-gray-500">N/A</span>}</span>
                        {spec?.image && <CopyButton value={spec.image} />}
                    </div>
                </DetailRow>

                <DetailRow label="Image SHA">
                    <div className="flex items-center gap-1">
                        <code className="text-xs bg-black/20 px-1.5 py-0.5 rounded font-mono">
                            {extractImageSha(status?.imageID)}
                        </code>
                        {status?.imageID && extractImageSha(status?.imageID) !== 'N/A' && (
                            <CopyButton value={extractImageSha(status?.imageID, true)} />
                        )}
                    </div>
                </DetailRow>

                <DetailRow label="Requests" value={formatResources(requests)} />

                <DetailRow label="Limits" value={formatResources(limits)} />

                <DetailRow label="Ports">
                    {ports.length > 0 ? (
                        <div className="flex flex-wrap gap-2">
                            {ports.map((port, idx) => {
                                const portStyle = getPortStyle(port.containerPort);
                                const config = getPortForwardConfig(port.containerPort);
                                const status = config ? getConfigStatus(config.id) : null;
                                const isRunning = status === 'running';

                                return (
                                    <div key={idx} className={`inline-flex items-center text-xs border rounded ${portStyle.className.replace(/hover:\S+/g, '')}`}>
                                        <button
                                            onClick={() => handlePortClick(port)}
                                            className={`inline-flex items-center gap-1.5 px-2 py-1 hover:bg-white/10 rounded-l transition-colors`}
                                            title={portStyle.title}
                                        >
                                            <SignalIcon className="w-3.5 h-3.5" />
                                            {port.containerPort}
                                            {port.protocol && port.protocol !== 'TCP' && `/${port.protocol}`}
                                            {port.name && <span className="opacity-60">({port.name})</span>}
                                        </button>
                                        {config && (
                                            <div className="flex items-center border-l border-inherit">
                                                <button
                                                    onClick={(e) => handleToggleForward(e, config)}
                                                    className={`p-1 hover:bg-white/10 transition-colors ${isRunning ? 'text-red-400' : 'text-green-400'}`}
                                                    title={isRunning ? 'Stop' : 'Start'}
                                                >
                                                    {isRunning ? (
                                                        <StopIcon className="w-3.5 h-3.5" />
                                                    ) : (
                                                        <PlayIcon className="w-3.5 h-3.5" />
                                                    )}
                                                </button>
                                                <button
                                                    onClick={(e) => handleOpenBrowser(e, config)}
                                                    className={`p-1 transition-colors ${isRunning ? 'hover:bg-white/10' : 'opacity-40 cursor-not-allowed'}`}
                                                    title={isRunning ? `Open ${config.https ? 'https' : 'http'}://localhost:${config.localPort}` : 'Start to open in browser'}
                                                    disabled={!isRunning}
                                                >
                                                    <ArrowTopRightOnSquareIcon className="w-3.5 h-3.5" />
                                                </button>
                                                <button
                                                    onClick={(e) => handleDeleteForward(e, config)}
                                                    className="p-1 hover:bg-white/10 text-red-400 transition-colors rounded-r"
                                                    title="Delete port forward"
                                                >
                                                    <TrashIcon className="w-3.5 h-3.5" />
                                                </button>
                                            </div>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    ) : (
                        <span className="text-gray-500">No ports exposed</span>
                    )}
                </DetailRow>

                {spec?.volumeMounts && spec.volumeMounts.length > 0 && (
                    <DetailRow label="Volumes">
                        <div className="flex flex-wrap gap-1.5">
                            {spec.volumeMounts.map((mount, idx) => (
                                <VolumeMountLabel key={idx} mount={mount} />
                            ))}
                        </div>
                    </DetailRow>
                )}

                {/* Additional container info */}
                {spec?.command && (
                    <DetailRow label="Command">
                        <code className="text-xs bg-black/20 px-1.5 py-0.5 rounded font-mono break-all">
                            {spec.command.join(' ')}
                        </code>
                    </DetailRow>
                )}

                {spec?.args && (
                    <DetailRow label="Args">
                        <code className="text-xs bg-black/20 px-1.5 py-0.5 rounded font-mono break-all">
                            {spec.args.join(' ')}
                        </code>
                    </DetailRow>
                )}

                {spec?.workingDir && (
                    <DetailRow label="Working Dir" value={spec.workingDir} />
                )}
            </div>

            {/* Port Forward Dialog */}
            <PodPortForwardDialog
                open={portForwardDialog.open}
                onOpenChange={handleClosePortForward}
                pod={pod}
                containerPort={portForwardDialog.port}
                currentContext={currentContext}
                existingConfig={portForwardDialog.existingConfig}
            />
        </div>
    );
}
