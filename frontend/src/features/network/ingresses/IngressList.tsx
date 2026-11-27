import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon, PlayIcon, StopIcon, ArrowPathIcon, ExclamationTriangleIcon, CheckCircleIcon, SignalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import IngressActionsMenu from './IngressActionsMenu';
import { useIngresses } from '~/hooks/resources';
import { useIngressActions } from './useIngressActions';
import { useIngressForward } from '~/hooks/useIngressForward';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteIngress, GetIngressYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { CopyableLabel } from '~/components/shared/DetailComponents';
import { ingressSemantics, ingressStatus, ingressClass, ingressHosts, ingressAddress, ingressPaths } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildIngressViewColumns, INGRESS_HIDDEN_BY_DEFAULT, INGRESS_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const INGRESS_INITIAL_SORT = { key: 'age', direction: 'desc' };
const INGRESS_VIEW_OPTIONS = { sort: INGRESS_INITIAL_SORT, columns: defaultViewColumns('ingresses', INGRESS_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function IngressList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { ingresses, loading, loadState, view } = useIngresses(currentContext, selectedNamespaces, isVisible, INGRESS_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useIngressActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Ingress',
        resourceType: 'ingresses',
        isNamespaced: true,
        deleteApi: DeleteIngress,
        getYamlApi: GetIngressYaml,

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

    const isOtherContext = isActive && !!forwardState.context && forwardState.context !== currentContext;

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
        } catch (err: any) {
            console.error('Failed to start ingress forward:', err);
        }
    }, [detectedController, startForward]);

    const handleStopForward = useCallback(async () => {
        try {
            await stopForward();
        } catch (err: any) {
            console.error('Failed to stop ingress forward:', err);
        }
    }, [stopForward]);

    const handleRefreshHostnames = useCallback(async () => {
        try {
            await refreshHostnames([]); // Empty = all namespaces
        } catch (err: any) {
            console.error('Failed to refresh hostnames:', err);
        }
    }, [refreshHostnames]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: ingressSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: ingressSemantics.namespace.getValue },
        {
            key: 'status',
            label: 'Status',
            render: (item: any) => {
                const { status, color } = ingressStatus(item);
                return <span className={color}>{status}</span>;
            },
            getValue: ingressSemantics.status.getValue
        },
        { key: 'class', label: 'Class', render: (item: any) => ingressClass(item), getValue: ingressSemantics.class.getValue },
        { key: 'hosts', label: 'Hosts', render: (item: any) => {
            const rules = item.spec?.rules || [];
            const hosts = rules.map((r: any) => r.host).filter(Boolean);
            if (hosts.length === 0) return '*';
            return <span className="flex flex-wrap gap-1">{hosts.map((h: string) => <CopyableLabel key={h} value={h} />)}</span>;
        }, getValue: ingressSemantics.hosts.getValue },
        { key: 'address', label: 'Address', render: (item: any) => ingressAddress(item), getValue: ingressSemantics.address.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: ingressSemantics.age.getValue },
        // Hidden by default columns
        {
            key: 'paths',
            label: 'Paths',
            defaultHidden: true,
            render: (item: any) => ingressPaths(item),
            getValue: ingressSemantics.paths.getValue,
        },
        {
            key: 'tls',
            label: 'TLS',
            defaultHidden: true,
            render: (item: any) => {
                const tls = item.spec?.tls || [];
                if (tls.length === 0) return <span className="text-gray-500">No</span>;
                return <span className="text-green-400">Yes ({tls.length})</span>;
            },
            getValue: ingressSemantics.tls.getValue,
        },
        {
            key: 'defaultBackend',
            label: 'Default Backend',
            defaultHidden: true,
            render: (item: any) => {
                const backend = item.spec?.defaultBackend;
                if (!backend) return <span className="text-gray-500">-</span>;
                const svc = backend.service;
                if (svc) return `${svc.name}:${svc.port?.number || svc.port?.name}`;
                return '-';
            },
            getValue: ingressSemantics.defaultBackend.getValue,
        },
        {
            key: 'rules',
            label: 'Rules',
            defaultHidden: true,
            render: (item: any) => (item.spec?.rules || []).length,
            getValue: ingressSemantics.rules.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={IngressActionsMenu}
                    menuId={`ingress-${item.metadata.uid}`}
                    ingress={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(ingress: any) => openBulkDelete([ingress])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildIngressViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

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
                ) : isOtherContext ? (
                    <>
                        <div className="flex items-center gap-1.5 text-xs text-amber-400">
                            <ExclamationTriangleIcon className="h-4 w-4" />
                            <span>
                                Forwarding active in cluster: <span className="font-mono">{forwardState.context}</span>
                            </span>
                        </div>
                        <button
                            onClick={handleStopForward}
                            className="flex items-center gap-1 px-2 py-1 text-xs text-red-400 hover:text-red-300 hover:bg-gray-700 rounded transition-colors"
                            disabled={forwardLoading}
                            title={`Stop forwarding in ${forwardState.context}`}
                        >
                            <StopIcon className="h-3.5 w-3.5" />
                            Stop
                        </button>
                    </>
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

                {!isOtherContext && forwardState.hostsFileUpdated && (
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
                                            {previewHostnames.map((hostname: any, i: number) => (
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
                                    <p>{(forwardError as any).message || String(forwardError)}</p>
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
                <ResourceList loadState={loadState}
                    title="Ingresses"
                    columns={view ? viewColumns : columns}
                    source={view ?? null}
                    viewPinnedColumns={INGRESS_PINNED_COLUMNS}
                    data={view ? NO_DATA : ingresses}
                    isLoading={loading}
                    namespaces={namespaces}
                    currentNamespace={selectedNamespaces}
                    onNamespaceChange={setSelectedNamespaces}
                    showNamespaceSelector={true}
                    multiSelectNamespaces={true}
                    initialSort={INGRESS_INITIAL_SORT}
                    resourceType="ingresses"
                    onRowClick={view ? showDetails : handleShowDetails}
                    selectable={true}
                    selection={selection}
                    onBulkDelete={openBulkDelete}
                />
            </div>
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </div>
    );
}
