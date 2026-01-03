import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import StatefulSetActionsMenu from './StatefulSetActionsMenu';
import { useStatefulSets, usePods } from '../../../hooks/resources';
import { useStatefulSetActions } from './useStatefulSetActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteStatefulSet, RestartStatefulSet, GetStatefulSetYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { getDeploymentPods, getEffectivePodStatus, getPodStatusColor } from '../../../utils/k8s-helpers';
import Logger from '../../../utils/Logger';

export default function StatefulSetList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    // Bulk action modal state
    const [bulkActionModal, setBulkActionModal] = useState({
        isOpen: false,
        action: null, // 'delete' | 'restart'
        items: [],
    });
    const [bulkProgress, setBulkProgress] = useState({
        current: 0,
        total: 0,
        status: 'idle',
        results: [],
    });

    // Handle bulk action button clicks
    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkRestartClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'restart', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    // Handle bulk action confirmation
    const handleBulkActionConfirm = useCallback(async (items) => {
        const action = bulkActionModal.action;
        Logger.info(`Bulk ${action} started`, { count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));

        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                if (action === 'delete') {
                    await DeleteStatefulSet(currentContext, namespace, name);
                } else if (action === 'restart') {
                    await RestartStatefulSet(currentContext, namespace, name);
                }
                results.push({ name, namespace, success: true, message: '' });
                Logger.info(`StatefulSet ${action}ed`, { namespace, name });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
                Logger.error(`Failed to ${action} statefulset`, { namespace, name, error: err });
            }

            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }

        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
        Logger.info(`Bulk ${action} completed`, {
            total: items.length,
            success: results.filter(r => r.success).length,
            failed: results.filter(r => !r.success).length,
        });
    }, [currentContext, bulkActionModal.action]);

    // Handle modal close
    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    // Handle YAML backup export
    const handleExportYaml = useCallback(async (items) => {
        Logger.info('Exporting YAML backup', { count: items.length });

        const entries = [];
        for (const item of items) {
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                const yaml = await GetStatefulSetYaml(namespace, name);
                entries.push({ namespace, name, kind: 'StatefulSet', yaml });
                Logger.info('Fetched YAML for backup', { namespace, name });
            } catch (err) {
                Logger.error('Failed to get YAML for backup', { namespace, name, error: err });
                entries.push({ namespace, name, kind: 'StatefulSet', yaml: `# Failed to fetch YAML: ${err}` });
            }
        }

        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        const defaultFilename = `statefulsets-backup-${timestamp}.zip`;

        try {
            await SaveYamlBackup(entries, defaultFilename);
            Logger.info('YAML backup saved');
        } catch (err) {
            Logger.error('Failed to save YAML backup', { error: err });
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
    // console.log("StatefulSetList rendering");
    const { statefulSets, loading: statefulSetsLoading } = useStatefulSets(currentContext, selectedNamespaces, isVisible);
    const { pods: allPods, loading: podsLoading } = usePods(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs } = useStatefulSetActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'pods',
            label: 'Pods',
            render: (item) => {
                if (podsLoading && allPods.length === 0) {
                    const count = item.spec?.replicas ?? 1;
                    if (count === 0) return null;
                    return (
                        <div className="flex gap-1">
                            {Array.from({ length: Math.min(count, 5) }).map((_, i) => (
                                <div
                                    key={i}
                                    className="w-3 h-3 rounded-sm bg-gray-700 animate-pulse"
                                    title="Loading pods..."
                                />
                            ))}
                            {count > 5 && <span className="text-xs text-gray-500">...</span>}
                        </div>
                    );
                }
                return (
                    <div className="flex gap-1">
                        {getDeploymentPods(item, allPods).map((pod) => {
                            const status = getEffectivePodStatus(pod);
                            const colorClass = getPodStatusColor(status).replace('text-', 'bg-');
                            return (
                                <div
                                    key={pod.metadata.uid}
                                    className={`w-3 h-3 rounded-sm ${colorClass}`}
                                    title={`${pod.metadata.name}: ${status}`}
                                />
                            );
                        })}
                    </div>
                );
            },
            getValue: (item) => getDeploymentPods(item, allPods).length
        },
        { key: 'ready', label: 'Ready', render: (item) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: (item) => item.status?.readyReplicas || 0 },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <StatefulSetActionsMenu
                    statefulSet={item}
                    isOpen={activeMenuId === `statefulset-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `statefulset-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onRestart={() => handleRestart(item)}
                    onDelete={() => handleDelete(item)}
                    onViewLogs={() => handleViewLogs(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs, podsLoading, allPods]);

    return (
        <>
            <ResourceList
                title="StatefulSets"
                columns={columns}
                data={statefulSets}
                isLoading={statefulSetsLoading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="statefulsets"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
                onBulkRestart={handleBulkRestartClick}
            />

            {/* Bulk Action Modal */}
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={handleBulkActionClose}
                action={bulkActionModal.action}
                actionLabel={bulkActionModal.action === 'delete' ? 'Delete' : 'Restart'}
                items={bulkActionModal.items}
                onConfirm={handleBulkActionConfirm}
                onExportYaml={bulkActionModal.action === 'delete' ? handleExportYaml : null}
                progress={bulkProgress}
            />
        </>
    );
}
