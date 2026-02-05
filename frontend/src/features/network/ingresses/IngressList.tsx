import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon, PlayIcon, StopIcon, ArrowPathIcon, ExclamationTriangleIcon, CheckCircleIcon, SignalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import IngressActionsMenu from './IngressActionsMenu';
import { useIngresses } from '../../../hooks/resources';
import { useIngressActions } from './useIngressActions';
import { useIngressForward } from '../../../hooks/useIngressForward';
import { useK8s } from '../../../context';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteIngress, GetIngressYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function IngressList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { ingresses, loading } = useIngresses(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useIngressActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Ingress',
        resourceType: 'ingresses',
        isNamespaced: true,
        deleteApi: DeleteIngress,
        getYamlApi: GetIngressYaml,
        currentContext,
    });

    // Ingress forwarding state
    const {
        state: forwardState,
        detectedController,
        detectionAttempted,
        previewHostnames,
        loading: forwardLoading,
        detecting,
        error: forwardError,
        isActive,
        isRunning,
        detectController,
        previewHosts,
        start: startForward,
        stop: stopForward,
        refreshHostnames,
        resetDetection
    } = useIngressForward();

    const [showForwardDialog, setShowForwardDialog] = useState(false);

    // Detect controller when dialog opens (only if not already attempted)
    // Always search ALL namespaces for ingresses, not just selected ones
    useEffect(() => {
        if (showForwardDialog && !detectionAttempted && !detecting) {
            detectController();
            previewHosts([]); // Empty array = all namespaces
        }
    }, [showForwardDialog, detectionAttempted, detecting, detectController, previewHosts]);

    // Reset detection state when dialog closes
    useEffect(() => {
        if (!showForwardDialog) {
            resetDetection();
        }
    }, [showForwardDialog, resetDetection]);

    const handleStartForward = useCallback(async () => {
        if (!detectedController) return;
        try {
            await startForward(detectedController, []); // Empty = all namespaces
            setShowForwardDialog(false);
        } catch (err) {
            console.error('Failed to start ingress forward:', err);
        }
    }, [detectedController, startForward]);

    const handleStopForward = useCallback(async () => {
        try {
            await stopForward();
        } catch (err) {
            console.error('Failed to stop ingress forward:', err);
        }
    }, [stopForward]);

    const handleRefreshHostnames = useCallback(async () => {
        try {
            await refreshHostnames([]); // Empty = all namespaces
        } catch (err) {
            console.error('Failed to refresh hostnames:', err);
        }
    }, [refreshHostnames]);

    const getHosts = (ingress) => {
        const rules = ingress.spec?.rules || [];
        const hosts = rules.map(r => r.host).filter(Boolean);
        return hosts.length > 0 ? hosts.join(', ') : '*';
    };

    const getPaths = (ingress) => {
        const rules = ingress.spec?.rules || [];
        const paths = [];
        for (const rule of rules) {
            const httpPaths = rule.http?.paths || [];
            for (const p of httpPaths) {
                paths.push(p.path || '/');
            }
        }
        return paths.length > 0 ? paths.slice(0, 3).join(', ') + (paths.length > 3 ? '...' : '') : '-';
    };

    const getIngressClass = (ingress) => {
        return ingress.spec?.ingressClassName || ingress.metadata?.annotations?.['kubernetes.io/ingress.class'] || '-';
    };

    const getAddress = (ingress) => {
        const lbIngress = ingress.status?.loadBalancer?.ingress || [];
        if (lbIngress.length === 0) return '-';
        const addresses = lbIngress.map(lb => lb.ip || lb.hostname).filter(Boolean);
        return addresses.length > 0 ? addresses.join(', ') : '-';
    };

    const getIngressStatus = (ingress) => {
        const lbIngress = ingress.status?.loadBalancer?.ingress || [];
        const rules = ingress.spec?.rules || [];
        const defaultBackend = ingress.spec?.defaultBackend;

        if (lbIngress.length > 0) {
            const hasAddress = lbIngress.some(lb => lb.ip || lb.hostname);
            if (hasAddress) return { status: 'Active', color: 'text-green-400' };
        }
        if (rules.length === 0 && !defaultBackend) {
            return { status: 'No Rules', color: 'text-red-400' };
        }
        return { status: 'Pending', color: 'text-yellow-400' };
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'status',
            label: 'Status',
            render: (item) => {
                const { status, color } = getIngressStatus(item);
                return <span className={color}>{status}</span>;
            },
            getValue: (item) => getIngressStatus(item).status
        },
        { key: 'class', label: 'Class', render: (item) => getIngressClass(item), getValue: (item) => getIngressClass(item) },
        { key: 'hosts', label: 'Hosts', render: (item) => getHosts(item), getValue: (item) => getHosts(item) },
        { key: 'address', label: 'Address', render: (item) => getAddress(item), getValue: (item) => getAddress(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        // Hidden by default columns
        {
            key: 'paths',
            label: 'Paths',
            defaultHidden: true,
            render: (item) => getPaths(item),
            getValue: (item) => getPaths(item),
        },
        {
            key: 'tls',
            label: 'TLS',
            defaultHidden: true,
            render: (item) => {
                const tls = item.spec?.tls || [];
                if (tls.length === 0) return <span className="text-gray-500">No</span>;
                return <span className="text-green-400">Yes ({tls.length})</span>;
            },
            getValue: (item) => (item.spec?.tls || []).length > 0 ? 'Yes' : 'No',
        },
        {
            key: 'defaultBackend',
            label: 'Default Backend',
            defaultHidden: true,
            render: (item) => {
                const backend = item.spec?.defaultBackend;
                if (!backend) return <span className="text-gray-500">-</span>;
                const svc = backend.service;
                if (svc) return `${svc.name}:${svc.port?.number || svc.port?.name}`;
                return '-';
            },
            getValue: (item) => item.spec?.defaultBackend?.service?.name || '',
        },
        {
            key: 'rules',
            label: 'Rules',
            defaultHidden: true,
            render: (item) => (item.spec?.rules || []).length,
            getValue: (item) => (item.spec?.rules || []).length,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <IngressActionsMenu
                    ingress={item}
                    isOpen={activeMenuId === `ingress-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `ingress-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(ingress) => openBulkDelete([ingress])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <div className="flex flex-col h-full">
            {/* Ingress Forward Controls */}
            <div className="flex items-center gap-2 px-4 py-2 bg-gray-800/50 border-b border-gray-700">
                <SignalIcon className="h-4 w-4 text-gray-400" />
                <span className="text-sm text-gray-400">Local Forward:</span>

                {!isActive ? (
                    <button
                        onClick={() => setShowForwardDialog(true)}
                        className="flex items-center gap-1.5 px-2.5 py-1 text-xs font-medium text-white bg-blue-600 hover:bg-blue-700 rounded transition-colors"
                        disabled={forwardLoading}
                    >
                        <PlayIcon className="h-3.5 w-3.5" />
                        Forward All
                    </button>
                ) : (
                    <>
                        <div className="flex items-center gap-1.5 text-xs">
                            {isRunning ? (
                                <CheckCircleIcon className="h-4 w-4 text-green-400" />
                            ) : forwardState.status === 'error' ? (
                                <ExclamationTriangleIcon className="h-4 w-4 text-red-400" />
                            ) : (
                                <div className="h-4 w-4 border-2 border-blue-400 border-t-transparent rounded-full animate-spin" />
                            )}
                            <span className={isRunning ? 'text-green-400' : forwardState.status === 'error' ? 'text-red-400' : 'text-blue-400'}>
                                {forwardState.status === 'running' && `Forwarding ${forwardState.hostnames?.length || 0} hosts`}
                                {forwardState.status === 'starting' && 'Starting...'}
                                {forwardState.status === 'error' && (forwardState.error || 'Error')}
                            </span>
                            {isRunning && forwardState.localHttpsPort > 0 && (
                                <span className="text-gray-500">
                                    (HTTPS:{forwardState.localHttpsPort}{forwardState.localHttpPort > 0 ? `, HTTP:${forwardState.localHttpPort}` : ''})
                                </span>
                            )}
                        </div>
                        <button
                            onClick={handleRefreshHostnames}
                            className="flex items-center gap-1 px-2 py-1 text-xs text-gray-300 hover:text-white hover:bg-gray-700 rounded transition-colors"
                            disabled={forwardLoading}
                            title="Refresh hostnames"
                        >
                            <ArrowPathIcon className={`h-3.5 w-3.5 ${forwardLoading ? 'animate-spin' : ''}`} />
                        </button>
                        <button
                            onClick={handleStopForward}
                            className="flex items-center gap-1 px-2 py-1 text-xs text-red-400 hover:text-red-300 hover:bg-gray-700 rounded transition-colors"
                            disabled={forwardLoading}
                        >
                            <StopIcon className="h-3.5 w-3.5" />
                            Stop
                        </button>
                    </>
                )}

                {forwardState.hostsFileUpdated && (
                    <span className="text-xs text-gray-500">(hosts file updated)</span>
                )}
            </div>

            {/* Forward Dialog */}
            {showForwardDialog && (
                <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60">
                    <div className="bg-gray-800 rounded-lg shadow-xl w-full max-w-lg mx-4 overflow-hidden">
                        <div className="px-4 py-3 border-b border-gray-700">
                            <h3 className="text-lg font-medium text-white">Forward All Ingresses</h3>
                        </div>
                        <div className="p-4 space-y-4">
                            {/* Controller Detection */}
                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-1">Ingress Controller</label>
                                {detecting ? (
                                    <div className="flex items-center gap-2 text-sm text-gray-400">
                                        <div className="h-4 w-4 border-2 border-blue-400 border-t-transparent rounded-full animate-spin" />
                                        Detecting...
                                    </div>
                                ) : detectedController ? (
                                    <div className="text-sm text-white bg-gray-700 px-3 py-2 rounded">
                                        {detectedController.type} - {detectedController.namespace}/{detectedController.name}
                                        <span className="text-gray-400 ml-2">
                                            (HTTP:{detectedController.httpPort}, HTTPS:{detectedController.httpsPort})
                                        </span>
                                    </div>
                                ) : (
                                    <div className="space-y-2">
                                        <div className="flex items-center justify-between">
                                            <div className="flex items-center gap-2 text-sm text-red-400">
                                                <ExclamationTriangleIcon className="h-4 w-4" />
                                                No ingress controller detected
                                            </div>
                                            <button
                                                onClick={() => {
                                                    resetDetection();
                                                    setTimeout(() => {
                                                        detectController();
                                                        previewHosts([]);
                                                    }, 0);
                                                }}
                                                className="text-xs text-blue-400 hover:text-blue-300"
                                            >
                                                Retry
                                            </button>
                                        </div>
                                        {forwardError && (
                                            <p className="text-xs text-gray-500">
                                                {forwardError.message || String(forwardError)}
                                            </p>
                                        )}
                                        <p className="text-xs text-gray-500">
                                            Looking for: traefik, ingress-nginx, contour, haproxy
                                        </p>
                                    </div>
                                )}
                            </div>

                            {/* Hostnames Preview */}
                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-1">
                                    Hostnames to add to /etc/hosts ({previewHostnames.length})
                                </label>
                                <div className="bg-gray-900 rounded max-h-48 overflow-y-auto">
                                    {previewHostnames.length > 0 ? (
                                        <ul className="text-xs text-gray-300 p-2 space-y-0.5">
                                            {previewHostnames.map((hostname, i) => (
                                                <li key={i} className="font-mono">127.0.0.1 {hostname}</li>
                                            ))}
                                        </ul>
                                    ) : (
                                        <p className="text-sm text-gray-500 p-3">No hostnames found in ingresses</p>
                                    )}
                                </div>
                            </div>

                            {/* Warning */}
                            <div className="flex items-start gap-2 text-xs text-yellow-400 bg-yellow-400/10 px-3 py-2 rounded">
                                <ExclamationTriangleIcon className="h-4 w-4 flex-shrink-0 mt-0.5" />
                                <p>This will modify your system's hosts file. You will be prompted for your password.</p>
                            </div>

                            {forwardError && (
                                <div className="flex items-start gap-2 text-xs text-red-400 bg-red-400/10 px-3 py-2 rounded">
                                    <ExclamationTriangleIcon className="h-4 w-4 flex-shrink-0 mt-0.5" />
                                    <p>{forwardError.message || forwardError}</p>
                                </div>
                            )}
                        </div>
                        <div className="flex justify-end gap-2 px-4 py-3 border-t border-gray-700 bg-gray-800/50">
                            <button
                                onClick={() => setShowForwardDialog(false)}
                                className="px-3 py-1.5 text-sm text-gray-300 hover:text-white hover:bg-gray-700 rounded transition-colors"
                            >
                                Cancel
                            </button>
                            <button
                                onClick={handleStartForward}
                                disabled={!detectedController || previewHostnames.length === 0 || forwardLoading}
                                className="flex items-center gap-1.5 px-3 py-1.5 text-sm font-medium text-white bg-blue-600 hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed rounded transition-colors"
                            >
                                {forwardLoading ? (
                                    <div className="h-4 w-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                                ) : (
                                    <PlayIcon className="h-4 w-4" />
                                )}
                                Start Forwarding
                            </button>
                        </div>
                    </div>
                </div>
            )}

            {/* Resource List */}
            <div className="flex-1 min-h-0">
                <ResourceList
                    title="Ingresses"
                    columns={columns}
                    data={ingresses}
                    isLoading={loading}
                    namespaces={namespaces}
                    currentNamespace={selectedNamespaces}
                    onNamespaceChange={setSelectedNamespaces}
                    showNamespaceSelector={true}
                    multiSelectNamespaces={true}
                    highlightedUid={activeMenuId}
                    initialSort={{ key: 'age', direction: 'desc' }}
                    resourceType="ingresses"
                    onRowClick={handleShowDetails}
                    selectable={true}
                    selection={selection}
                    onBulkDelete={openBulkDelete}
                />
            </div>
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </div>
    );
}
