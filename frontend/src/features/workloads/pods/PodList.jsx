import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import ResourceBar from '../../../components/shared/ResourceBar';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import PodActionsMenu from './PodActionsMenu';
import { usePods } from '../../../hooks/resources';
import { usePodMetrics } from '../../../hooks/usePodMetrics';
import { usePodActions } from './usePodActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeletePod, GetPodYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { getPodStatus, getPodStatusColor, getContainerStatusColor, getPodStatusPriority, getPodController } from '../../../utils/k8s-helpers';
import Logger from '../../../utils/Logger';

export default function PodList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId, navigateWithSearch } = useUI();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    // Bulk action modal state
    const [bulkDeleteModal, setBulkDeleteModal] = useState({
        isOpen: false,
        items: [],
    });
    const [bulkProgress, setBulkProgress] = useState({
        current: 0,
        total: 0,
        status: 'idle', // 'idle' | 'inProgress' | 'complete'
        results: [],
    });

    // Handle bulk delete button click
    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkDeleteModal({
            isOpen: true,
            items: selectedItems,
        });
        setBulkProgress({
            current: 0,
            total: selectedItems.length,
            status: 'idle',
            results: [],
        });
    }, []);

    // Handle bulk delete confirmation
    const handleBulkDeleteConfirm = useCallback(async (items) => {
        Logger.info('Bulk delete started', { count: items.length });
        setBulkProgress(prev => ({
            ...prev,
            status: 'inProgress',
            results: [],
        }));

        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                await DeletePod(currentContext, namespace, name);
                results.push({
                    name,
                    namespace,
                    success: true,
                    message: '',
                });
                Logger.info('Pod deleted', { namespace, name });
            } catch (err) {
                results.push({
                    name,
                    namespace,
                    success: false,
                    message: err.toString(),
                });
                Logger.error('Failed to delete pod', { namespace, name, error: err });
            }

            setBulkProgress(prev => ({
                ...prev,
                current: i + 1,
                results: [...results],
            }));
        }

        setBulkProgress(prev => ({
            ...prev,
            status: 'complete',
        }));
        Logger.info('Bulk delete completed', {
            total: items.length,
            success: results.filter(r => r.success).length,
            failed: results.filter(r => !r.success).length,
        });
    }, [currentContext]);

    // Handle modal close
    const handleBulkDeleteClose = useCallback(() => {
        setBulkDeleteModal({ isOpen: false, items: [] });
        setBulkProgress({
            current: 0,
            total: 0,
            status: 'idle',
            results: [],
        });
    }, []);

    // Handle YAML backup export with native save dialog
    const handleExportYaml = useCallback(async (items) => {
        Logger.info('Exporting YAML backup', { count: items.length });

        // Fetch YAML for each item
        const entries = [];
        for (const item of items) {
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                const yaml = await GetPodYaml(namespace, name);
                entries.push({
                    namespace,
                    name,
                    kind: 'Pod',
                    yaml,
                });
                Logger.info('Fetched YAML for backup', { namespace, name });
            } catch (err) {
                Logger.error('Failed to get YAML for backup', { namespace, name, error: err });
                // Add error entry
                entries.push({
                    namespace,
                    name,
                    kind: 'Pod',
                    yaml: `# Failed to fetch YAML: ${err}`,
                });
            }
        }

        // Save with native dialog
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        const defaultFilename = `pods-backup-${timestamp}.zip`;

        try {
            await SaveYamlBackup(entries, defaultFilename);
            Logger.info('YAML backup saved');
        } catch (err) {
            Logger.error('Failed to save YAML backup', { error: err });
            // Don't alert if user cancelled (empty error)
            if (err && err.toString() !== '') {
                alert('Failed to save backup: ' + err);
            }
        }
    }, []);

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            setMenuPosition({
                top: rect.bottom + 4,
                left: rect.right - 192
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId]);
    // console.log("PodList rendering");
    const { pods, loading } = usePods(currentContext, selectedNamespaces, isVisible);
    const { metrics, available: metricsAvailable } = usePodMetrics(isVisible);
    const { openLogs, handleShell, handleEditYaml, handleShowDependencies, handleShowDetails, handleDelete } = usePodActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'cpu',
            label: 'CPU',
            render: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                const m = metrics[key];
                if (!metricsAvailable) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <div className="flex flex-col gap-0.5">
                        <ResourceBar percent={m.cpuPercent} label="" tooltipLabel="Used" color="bg-blue-500" />
                        <ResourceBar percent={m.cpuCommittedPercent} label="" tooltipLabel="Committed" color="bg-red-500" fixedColor />
                    </div>
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
                if (!metricsAvailable) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <div className="flex flex-col gap-0.5">
                        <ResourceBar percent={m.memPercent} label="" tooltipLabel="Used" color="bg-purple-500" />
                        <ResourceBar percent={m.memCommittedPercent} label="" tooltipLabel="Committed" color="bg-red-500" fixedColor />
                    </div>
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

                // Map controller kind to view name
                const kindToView = {
                    'ReplicaSet': 'replicasets',
                    'Deployment': 'deployments',
                    'StatefulSet': 'statefulsets',
                    'DaemonSet': 'daemonsets',
                    'Job': 'jobs',
                    'CronJob': 'cronjobs',
                };
                const viewName = kindToView[controller.kind];

                if (viewName) {
                    return (
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                navigateWithSearch(viewName, `uid:"${controller.uid}"`);
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
                    onShell={() => handleShell(item.metadata.namespace, item.metadata.name)}
                    onDelete={() => handleDelete(item.metadata.namespace, item.metadata.name, false)}
                    onForceDelete={() => handleDelete(item.metadata.namespace, item.metadata.name, true)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onShowDetails={() => handleShowDetails(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, openLogs, handleShell, handleDelete, handleEditYaml, handleShowDependencies, handleShowDetails, pods, navigateWithSearch, metrics, metricsAvailable]);

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
                onBulkDelete={handleBulkDeleteClick}
            />

            {/* Bulk Delete Modal */}
            <BulkActionModal
                isOpen={bulkDeleteModal.isOpen}
                onClose={handleBulkDeleteClose}
                action="delete"
                actionLabel="Delete"
                items={bulkDeleteModal.items}
                onConfirm={handleBulkDeleteConfirm}
                onExportYaml={handleExportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
