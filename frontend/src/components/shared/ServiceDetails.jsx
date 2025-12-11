import React, { useState, useMemo, useCallback } from 'react';
import { LockClosedIcon, SignalIcon, ClipboardDocumentIcon, CheckIcon, PlayIcon, StopIcon, TrashIcon, ArrowTopRightOnSquareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { usePortForwards } from '../../hooks/usePortForwards';
import { useUI } from '../../context/UIContext';
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime';
import ServicePortForwardDialog from './ServicePortForwardDialog';

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

// Copyable label component
const CopyableLabel = ({ value, className = '' }) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
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
            className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded border transition-colors cursor-pointer ${
                copied
                    ? 'bg-green-500/20 text-green-400 border-green-500/30'
                    : `bg-gray-500/10 hover:bg-gray-500/20 text-gray-300 border-gray-500/30 ${className}`
            }`}
            title={copied ? 'Copied!' : 'Click to copy'}
        >
            {copied ? (
                <>
                    <CheckIcon className="w-3 h-3" />
                    Copied
                </>
            ) : (
                value
            )}
        </button>
    );
};

// Detail row component
const DetailRow = ({ label, value, children }) => (
    <div className="flex py-2 border-b border-border/50">
        <div className="w-40 text-xs font-medium text-gray-500 uppercase tracking-wider shrink-0">
            {label}
        </div>
        <div className="flex-1 text-sm text-gray-200">
            {children || value || <span className="text-gray-500">N/A</span>}
        </div>
    </div>
);

export default function ServiceDetails({ service, onClose, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { configs, activeForwards, startForward, stopForward, deleteConfig } = usePortForwards(currentContext, true);
    const { openModal, closeModal } = useUI();
    const [portForwardDialog, setPortForwardDialog] = useState({ open: false, port: null });

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    // Service data
    const spec = service.spec || {};
    const status = service.status || {};
    const ports = spec.ports || [];
    const clusterIPs = spec.clusterIPs || (spec.clusterIP ? [spec.clusterIP] : []);
    const externalIPs = spec.externalIPs || [];
    const loadBalancerIngress = status.loadBalancer?.ingress || [];
    const selector = spec.selector || {};

    // Find port forward config for a specific port
    const getPortForwardConfig = useCallback((port) => {
        return configs.find(c =>
            c.resourceType === 'service' &&
            c.resourceName === service.metadata?.name &&
            c.namespace === service.metadata?.namespace &&
            c.remotePort === port
        );
    }, [configs, service.metadata?.name, service.metadata?.namespace]);

    // Get status for a config ID from activeForwards
    const getConfigStatus = useCallback((configId) => {
        const af = activeForwards.find(af => af.config?.id === configId);
        return af?.status || 'stopped';
    }, [activeForwards]);

    // Get styling for a port based on port forward status
    const getPortStyle = useCallback((port) => {
        const config = getPortForwardConfig(port);
        if (!config) {
            return {
                className: 'bg-gray-500/10 hover:bg-gray-500/20 text-gray-400 border-gray-500/30',
                title: 'Click to create port forward'
            };
        }
        const status = getConfigStatus(config.id);
        switch (status) {
            case 'running':
                return {
                    className: 'bg-green-500/10 hover:bg-green-500/20 text-green-400 border-green-500/30',
                    title: 'Port forward running - click to manage'
                };
            case 'error':
                return {
                    className: 'bg-red-500/10 hover:bg-red-500/20 text-red-400 border-red-500/30',
                    title: 'Port forward error - click to manage'
                };
            default:
                return {
                    className: 'bg-primary/10 hover:bg-primary/20 text-primary border-primary/30',
                    title: 'Port forward configured - click to manage'
                };
        }
    }, [getPortForwardConfig, getConfigStatus]);

    // Handle port click - open dialog
    const handlePortClick = useCallback((port) => {
        const existingConfig = getPortForwardConfig(port.port);
        setPortForwardDialog({
            open: true,
            port: port,
            existingConfig: existingConfig || null
        });
    }, [getPortForwardConfig]);

    const handleClosePortForward = useCallback(() => {
        setPortForwardDialog({ open: false, port: null, existingConfig: null });
    }, []);

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

    return (
        <div className="flex flex-col h-full bg-[#1e1e1e]">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-red-900/30 border-b border-red-500/50 text-red-400 shrink-0">
                    <LockClosedIcon className="h-5 w-5" />
                    <span className="text-sm">
                        This service is from context <span className="font-medium">{tabContext}</span>.
                    </span>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {service.metadata?.namespace}/{service.metadata?.name}
                    </div>
                </div>
                <div className="flex items-center gap-2">
                    <button
                        onClick={onClose}
                        className="px-3 py-1.5 text-xs font-medium text-gray-400 hover:text-text hover:bg-white/5 rounded transition-colors"
                    >
                        Close
                    </button>
                </div>
            </div>

            {/* Content Area */}
            <div className="flex-1 overflow-auto p-4">
                <div className="bg-surface rounded-lg border border-border p-4">
                    {/* Type */}
                    <DetailRow label="Type">
                        <span className="px-2 py-0.5 text-xs rounded bg-primary/10 text-primary border border-primary/30">
                            {spec.type || 'ClusterIP'}
                        </span>
                    </DetailRow>

                    {/* Ports */}
                    <DetailRow label="Ports">
                        {ports.length > 0 ? (
                            <div className="flex flex-wrap gap-2">
                                {ports.map((port, idx) => {
                                    const portStyle = getPortStyle(port.port);
                                    const config = getPortForwardConfig(port.port);
                                    const status = config ? getConfigStatus(config.id) : null;
                                    const isRunning = status === 'running';

                                    return (
                                        <div key={idx} className={`inline-flex items-center text-xs border rounded ${portStyle.className.replace(/hover:\S+/g, '')}`}>
                                            <button
                                                onClick={() => handlePortClick(port)}
                                                className="inline-flex items-center gap-1.5 px-2 py-1 hover:bg-white/10 rounded-l transition-colors"
                                                title={portStyle.title}
                                            >
                                                <SignalIcon className="w-3.5 h-3.5" />
                                                {port.port}
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
                            <span className="text-gray-500">No ports defined</span>
                        )}
                    </DetailRow>

                    {/* Cluster IPs */}
                    <DetailRow label="Cluster IPs">
                        {clusterIPs.length > 0 && clusterIPs[0] !== 'None' ? (
                            <div className="flex flex-wrap gap-2">
                                {clusterIPs.map((ip, idx) => (
                                    <CopyableLabel key={idx} value={ip} />
                                ))}
                            </div>
                        ) : (
                            <span className="text-gray-500">{spec.clusterIP === 'None' ? 'None (Headless)' : 'N/A'}</span>
                        )}
                    </DetailRow>

                    {/* External IPs (for ExternalName or manually set) */}
                    {externalIPs.length > 0 && (
                        <DetailRow label="External IPs">
                            <div className="flex flex-wrap gap-2">
                                {externalIPs.map((ip, idx) => (
                                    <CopyableLabel key={idx} value={ip} />
                                ))}
                            </div>
                        </DetailRow>
                    )}

                    {/* LoadBalancer Ingress */}
                    {spec.type === 'LoadBalancer' && (
                        <DetailRow label="Load Balancer">
                            {loadBalancerIngress.length > 0 ? (
                                <div className="flex flex-wrap gap-2">
                                    {loadBalancerIngress.map((ingress, idx) => (
                                        <CopyableLabel
                                            key={idx}
                                            value={ingress.ip || ingress.hostname}
                                        />
                                    ))}
                                </div>
                            ) : (
                                <span className="text-yellow-400">Pending...</span>
                            )}
                        </DetailRow>
                    )}

                    {/* NodePort (for NodePort or LoadBalancer types) */}
                    {(spec.type === 'NodePort' || spec.type === 'LoadBalancer') && ports.some(p => p.nodePort) && (
                        <DetailRow label="Node Ports">
                            <div className="flex flex-wrap gap-2">
                                {ports.filter(p => p.nodePort).map((port, idx) => (
                                    <span key={idx} className="px-2 py-0.5 text-xs rounded bg-gray-500/10 text-gray-300 border border-gray-500/30">
                                        {port.nodePort}
                                        {port.name && <span className="opacity-60 ml-1">({port.name})</span>}
                                    </span>
                                ))}
                            </div>
                        </DetailRow>
                    )}

                    {/* ExternalName */}
                    {spec.type === 'ExternalName' && spec.externalName && (
                        <DetailRow label="External Name">
                            <div className="flex items-center gap-1">
                                <code className="text-xs bg-black/20 px-1.5 py-0.5 rounded font-mono">
                                    {spec.externalName}
                                </code>
                                <CopyButton value={spec.externalName} />
                            </div>
                        </DetailRow>
                    )}

                    {/* Internal Traffic Policy */}
                    <DetailRow label="Internal Traffic" value={spec.internalTrafficPolicy || 'Cluster'} />

                    {/* External Traffic Policy (for NodePort/LoadBalancer) */}
                    {(spec.type === 'NodePort' || spec.type === 'LoadBalancer') && (
                        <DetailRow label="External Traffic" value={spec.externalTrafficPolicy || 'Cluster'} />
                    )}

                    {/* Session Affinity */}
                    <DetailRow label="Session Affinity" value={spec.sessionAffinity || 'None'} />

                    {/* Selector */}
                    <DetailRow label="Selector">
                        {Object.keys(selector).length > 0 ? (
                            <div className="flex flex-wrap gap-2">
                                {Object.entries(selector).map(([key, value], idx) => (
                                    <CopyableLabel key={idx} value={`${key}=${value}`} />
                                ))}
                            </div>
                        ) : (
                            <span className="text-gray-500">No selector</span>
                        )}
                    </DetailRow>
                </div>
            </div>

            {/* Port Forward Dialog */}
            <ServicePortForwardDialog
                open={portForwardDialog.open}
                onOpenChange={handleClosePortForward}
                service={service}
                servicePort={portForwardDialog.port}
                currentContext={currentContext}
                existingConfig={portForwardDialog.existingConfig}
            />
        </div>
    );
}
