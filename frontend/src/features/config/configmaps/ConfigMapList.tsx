import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useConfigMaps } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { useMenu } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { DeleteConfigMap, GetConfigMapYaml, SaveYamlBackup } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ConfigMapActionsMenu from './ConfigMapActionsMenu';
import { useConfigMapActions } from './useConfigMapActions';
import Logger from '~/utils/Logger';
import { useMenuPosition } from '~/hooks/useMenuPosition';

// System ConfigMaps auto-created by Kubernetes in every namespace
const SYSTEM_CONFIGMAP_NAMES = ['kube-root-ca.crt'];

export default function ConfigMapList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { addNotification } = useNotification();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();
    const [hideSystemConfigMaps, setHideSystemConfigMaps] = useState(true);

    const [bulkActionModal, setBulkActionModal] = useState<any>({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState<any>({ current: 0, total: 0, status: 'idle', results: [] });

    const handleBulkDeleteClick = useCallback((selectedItems: any) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkActionConfirm = useCallback(async (items: any) => {
        Logger.info('Bulk delete started', { count: items.length }, 'config');
        setBulkProgress((prev: any) => ({ ...prev, status: 'inProgress', results: [] }));

        const results: any[] = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                await DeleteConfigMap(namespace, name);
                results.push({ name, namespace, success: true, message: '' });
                Logger.info('ConfigMap deleted', { namespace, name }, 'config');
            } catch (err: any) {
                results.push({ name, namespace, success: false, message: err.toString() });
                Logger.error('Failed to delete configmap', { namespace, name, error: err }, 'config');
            }

            setBulkProgress((prev: any) => ({ ...prev, current: i + 1, results: [...results] }));
        }

        setBulkProgress((prev: any) => ({ ...prev, status: 'complete' }));
    }, [currentContext]);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items: any, { onProgress, signal }: any = {}) => {
        Logger.info('Exporting YAML backup', { count: items.length }, 'config');
        const entries = [];
        for (let i = 0; i < items.length; i++) {
            if (signal?.aborted) break;
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;
            try {
                const yaml = await GetConfigMapYaml(namespace, name);
                entries.push({ namespace, name, kind: 'ConfigMap', yaml });
            } catch (err: any) {
                entries.push({ namespace, name, kind: 'ConfigMap', yaml: `# Failed to fetch YAML: ${err}` });
            }
            onProgress?.(i + 1, items.length);
        }
        if (entries.length === 0) return;
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try {
            await SaveYamlBackup(entries, `configmaps-backup-${timestamp}.zip`);
        } catch (err: any) {
            if (err && err.toString() !== '') addNotification({ type: 'error', title: 'Failed to save backup', message: String(err) });
        }
    }, []);
    const { configMaps, loading } = useConfigMaps(currentContext, selectedNamespaces, isVisible) as any;
    const { handleEditYaml, handleEditKeyValue, handleShowDependencies, handleDelete } = useConfigMapActions();

    // Filter out system ConfigMaps if toggle is enabled
    const filteredConfigMaps = useMemo(() => {
        if (!hideSystemConfigMaps) return configMaps;
        return configMaps.filter((cm: any) => !SYSTEM_CONFIGMAP_NAMES.includes(cm.metadata?.name));
    }, [configMaps, hideSystemConfigMaps]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        { key: 'keys', label: 'Keys', render: (item: any) => Object.keys(item.data || {}).join(', '), getValue: (item: any) => Object.keys(item.data || {}).join(', ') },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        // Hidden by default columns
        {
            key: 'keyCount',
            label: 'Key Count',
            defaultHidden: true,
            render: (item: any) => Object.keys(item.data || {}).length,
            getValue: (item: any) => Object.keys(item.data || {}).length,
        },
        {
            key: 'binaryKeys',
            label: 'Binary Keys',
            defaultHidden: true,
            render: (item: any) => {
                const count = Object.keys(item.binaryData || {}).length;
                return count > 0 ? count : <span className="text-gray-500">-</span>;
            },
            getValue: (item: any) => Object.keys(item.binaryData || {}).length,
        },
        {
            key: 'size',
            label: 'Size',
            defaultHidden: true,
            render: (item: any) => {
                const dataSize = JSON.stringify(item.data || {}).length;
                const binarySize = JSON.stringify(item.binaryData || {}).length;
                const total = dataSize + binarySize;
                if (total < 1024) return `${total} B`;
                return `${(total / 1024).toFixed(1)} KB`;
            },
            getValue: (item: any) => JSON.stringify(item.data || {}).length + JSON.stringify(item.binaryData || {}).length,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <ConfigMapActionsMenu
                    configMap={item}
                    isOpen={activeMenuId === `configmap-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `configmap-${item.metadata.uid}`, buttonElement)}
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

    const systemToggle = (
        <label
            className="flex items-center gap-2 text-sm text-gray-400 hover:text-gray-300 cursor-pointer select-none no-drag"
            title="Hide kube-root-ca.crt and other system-managed ConfigMaps"
        >
            <input
                type="checkbox"
                checked={hideSystemConfigMaps}
                onChange={(e: any) => setHideSystemConfigMaps(e.target.checked)}
            />
            <span className="whitespace-nowrap">Hide system</span>
        </label>
    );

    return (
        <>
            <ResourceList
                title="ConfigMaps"
                columns={columns}
                data={filteredConfigMaps}
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
                customHeaderActions={systemToggle}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={handleBulkActionClose}
                action={bulkActionModal.action || ''}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={handleBulkActionConfirm}
                onExportYaml={handleExportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
