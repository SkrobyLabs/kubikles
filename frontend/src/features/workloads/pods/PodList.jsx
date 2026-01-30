import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import AggregateResourceBar from '../../../components/shared/AggregateResourceBar';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import PodActionsMenu from './PodActionsMenu';
import { usePods } from '../../../hooks/resources';
import { usePodMetrics } from '../../../hooks/usePodMetrics';
import { usePodActions } from './usePodActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeletePod, GetPodYaml } from '../../../../wailsjs/go/main/App';
import { formatAge, formatBytes, formatCpu } from '../../../utils/formatting';
import { getPodStatus, getPodStatusColor, getContainerStatusColor, getPodStatusPriority, getPodController } from '../../../utils/k8s-helpers';
import { getOwnerViewId } from '../../../utils/owner-navigation';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function PodList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for owner reference resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Pod',
        resourceType: 'pods',
        isNamespaced: true,
        deleteApi: DeletePod,
        getYamlApi: GetPodYaml,
        currentContext,
    });
    const { pods, loading } = usePods(currentContext, selectedNamespaces, isVisible);
    // Delay metrics fetch until pods are loaded to prioritize showing pod list first
    const { metrics, available: metricsAvailable } = usePodMetrics(isVisible, !loading && pods.length > 0);
    const { openLogs, handleShell, handleFiles, handleEditYaml, handleShowDependencies, handleShowDetails } = usePodActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'cpu',
            label: 'CPU',
            render: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                const m = metrics[key];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.cpuPercent}
                        reservedPercent={m.cpuReservedPercent}
                        committedPercent={m.cpuCommittedPercent}
                        type="cpu"
                        label="CPU"
                        usageValue={m.cpuUsage}
                        reservedValue={m.cpuRequested}
                        committedValue={m.cpuCommitted}
                        capacityValue={m.nodeCpuCapacity}
                        formatValue={formatCpu}
                    />
                );
            },
            getValue: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                return metrics[key]?.cpuCommittedPercent ?? -1;
            }
        },
        {
            key: 'memory',
            label: 'Memory',
            render: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                const m = metrics[key];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.memPercent}
                        reservedPercent={m.memReservedPercent}
                        committedPercent={m.memCommittedPercent}
                        type="memory"
                        label="Memory"
                        usageValue={m.memoryUsage}
                        reservedValue={m.memRequested}
                        committedValue={m.memCommitted}
                        capacityValue={m.nodeMemCapacity}
                        formatValue={formatBytes}
                    />
                );
            },
            getValue: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                return metrics[key]?.memCommittedPercent ?? -1;
            }
        },
        {
            key: 'containers',
            label: 'Containers',
            render: (item) => (
                <div className="flex gap-1">
                    {(item.status?.containerStatuses || []).map((status, i) => (
                        <div
                            key={i}
                            className={`w-3 h-3 rounded-sm ${getContainerStatusColor(status)}`}
                            title={`${status.name}: ${Object.keys(status.state || {})[0]} (${status.state?.waiting?.reason || ''})`}
                        />
                    ))}
                </div>
            ),
            getValue: (item) => getPodStatusPriority(getPodStatus(item))
        },
        {
            key: 'status',
            label: 'Status',
            render: (item) => {
                const status = getPodStatus(item);
                const colorClass = getPodStatusColor(status);
                return <span className={`font-medium ${colorClass}`}>{status}</span>;
            },
            getValue: (item) => getPodStatusPriority(getPodStatus(item))
        },
        { key: 'restarts', label: 'Restarts', render: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0, getValue: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0 },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item) => {
                const controller = getPodController(item);
                if (!controller) {
                    return <span className="text-gray-600">-</span>;
                }

                const viewId = getOwnerViewId(controller, crds);

                if (viewId) {
                    return (
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                navigateWithSearch(viewId, `uid:"${controller.uid}"`);
                            }}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors"
                            title={`Go to ${controller.kind}: ${controller.name}`}
                        >
                            {controller.kind}
                        </button>
                    );
                }

                return (
                    <span className="text-gray-400" title={controller.name}>
                        {controller.kind}
                    </span>
                );
            },
            getValue: (item) => getPodController(item)?.kind || ''
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PodActionsMenu
                    pod={item}
                    isOpen={activeMenuId === `pod-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `pod-${item.metadata.uid}`, buttonElement)}
                    onLogs={() => {
                        const containers = [
                            ...(item.spec?.initContainers || []).map(c => c.name),
                            ...(item.spec?.containers || []).map(c => c.name)
                        ];

                        const controller = getPodController(item);
                        let siblingPods = [];
                        if (controller) {
                            siblingPods = pods
                                .filter(p => {
                                    const c = getPodController(p);
                                    return c && c.uid === controller.uid;
                                })
                                .map(p => p.metadata.name);
                        } else {
                            // If no controller, just show itself
                            siblingPods = [item.metadata.name];
                        }

                        openLogs(item.metadata.namespace, item.metadata.name, containers, siblingPods, {}, '', item.metadata.creationTimestamp);
                    }}
                    onShell={() => handleShell(item)}
                    onFiles={() => handleFiles(item)}
                    onDelete={() => openBulkDelete([item])}
                    onForceDelete={() => openBulkDelete([item])}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onShowDetails={() => handleShowDetails(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, openLogs, handleShell, handleFiles, openBulkDelete, handleEditYaml, handleShowDependencies, handleShowDetails, pods, navigateWithSearch, metrics, metricsAvailable, crds]);

    return (
        <>
            <ResourceList
                title="Pods"
                columns={columns}
                data={pods}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="pods"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />

            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action="delete"
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
