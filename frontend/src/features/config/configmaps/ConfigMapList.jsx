import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import { useConfigMaps } from '../../../hooks/resources';
import { useK8s } from '../../../context/K8sContext';
import { useNotification } from '../../../context/NotificationContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteConfigMap, GetConfigMapYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ConfigMapActionsMenu from './ConfigMapActionsMenu';
import { useConfigMapActions } from './useConfigMapActions';
import Logger from '../../../utils/Logger';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function ConfigMapList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { addNotification } = useNotification();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    const [bulkActionModal, setBulkActionModal] = useState({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState({ current: 0, total: 0, status: 'idle', results: [] });

    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkActionConfirm = useCallback(async (items) => {
        Logger.info('Bulk delete started', { count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));

        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                await DeleteConfigMap(namespace, name);
                results.push({ name, namespace, success: true, message: '' });
                Logger.info('ConfigMap deleted', { namespace, name });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
                Logger.error('Failed to delete configmap', { namespace, name, error: err });
            }

            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }

        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, [currentContext]);

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
                const yaml = await GetConfigMapYaml(namespace, name);
                entries.push({ namespace, name, kind: 'ConfigMap', yaml });
            } catch (err) {
                entries.push({ namespace, name, kind: 'ConfigMap', yaml: `# Failed to fetch YAML: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try {
            await SaveYamlBackup(entries, `configmaps-backup-${timestamp}.zip`);
        } catch (err) {
            if (err && err.toString() !== '') addNotification({ type: 'error', title: 'Failed to save backup', message: String(err) });
        }
    }, []);
    const { configMaps, loading } = useConfigMaps(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleEditKeyValue, handleShowDependencies, handleDelete } = useConfigMapActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'keys', label: 'Keys', render: (item) => Object.keys(item.data || {}).join(', '), getValue: (item) => Object.keys(item.data || {}).join(', ') },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ConfigMapActionsMenu
                    configMap={item}
                    isOpen={activeMenuId === `configmap-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `configmap-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleDelete]);

    return (
        <>
            <ResourceList
                title="ConfigMaps"
                columns={columns}
                data={configMaps}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="configmaps"
                onRowClick={handleEditKeyValue}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={handleBulkActionClose}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={handleBulkActionConfirm}
                onExportYaml={handleExportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
