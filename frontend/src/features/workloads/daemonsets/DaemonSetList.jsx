import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import DaemonSetActionsMenu from './DaemonSetActionsMenu';
import { useDaemonSets } from '../../../hooks/resources';
import { useDaemonSetActions } from './useDaemonSetActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteDaemonSet, RestartDaemonSet, GetDaemonSetYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

export default function DaemonSetList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    // Bulk action modal state
    const [bulkActionModal, setBulkActionModal] = useState({
        isOpen: false,
        action: null,
        items: [],
    });
    const [bulkProgress, setBulkProgress] = useState({
        current: 0,
        total: 0,
        status: 'idle',
        results: [],
    });

    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkRestartClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'restart', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

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
                    await DeleteDaemonSet(currentContext, namespace, name);
                } else if (action === 'restart') {
                    await RestartDaemonSet(currentContext, namespace, name);
                }
                results.push({ name, namespace, success: true, message: '' });
                Logger.info(`DaemonSet ${action}ed`, { namespace, name });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
                Logger.error(`Failed to ${action} daemonset`, { namespace, name, error: err });
            }

            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }

        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, [currentContext, bulkActionModal.action]);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        Logger.info('Exporting YAML backup', { count: items.length });

        const entries = [];
        for (const item of items) {
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                const yaml = await GetDaemonSetYaml(namespace, name);
                entries.push({ namespace, name, kind: 'DaemonSet', yaml });
            } catch (err) {
                Logger.error('Failed to get YAML for backup', { namespace, name, error: err });
                entries.push({ namespace, name, kind: 'DaemonSet', yaml: `# Failed to fetch YAML: ${err}` });
            }
        }

        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        const defaultFilename = `daemonsets-backup-${timestamp}.zip`;

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
    const { daemonSets, loading } = useDaemonSets(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs } = useDaemonSetActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'desired',
            label: 'Desired',
            render: (item) => item.status?.desiredNumberScheduled || 0,
            getValue: (item) => item.status?.desiredNumberScheduled || 0
        },
        {
            key: 'current',
            label: 'Current',
            render: (item) => item.status?.currentNumberScheduled || 0,
            getValue: (item) => item.status?.currentNumberScheduled || 0
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item) => item.status?.numberReady || 0,
            getValue: (item) => item.status?.numberReady || 0
        },
        {
            key: 'available',
            label: 'Available',
            render: (item) => item.status?.numberAvailable || 0,
            getValue: (item) => item.status?.numberAvailable || 0
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <DaemonSetActionsMenu
                    daemonSet={item}
                    isOpen={activeMenuId === `ds-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `ds-${item.metadata.uid}`, buttonElement)}
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
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleRestart, handleDelete, handleViewLogs]);

    return (
        <>
            <ResourceList
                title="DaemonSets"
                columns={columns}
                data={daemonSets}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="daemonsets"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
                onBulkRestart={handleBulkRestartClick}
            />
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
