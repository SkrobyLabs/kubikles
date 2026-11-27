import React, { useMemo, useState, useCallback, useRef } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
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
import { sleep } from '~/utils/bulkExecutor';
import { configMapSemantics } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildConfigMapViewColumns } from './viewColumns';

// System ConfigMaps auto-created by Kubernetes in every namespace
const SYSTEM_CONFIGMAP_NAMES = ['kube-root-ca.crt'];

const CONFIGMAP_INITIAL_SORT = { key: 'age', direction: 'desc' };
const CONFIGMAP_HIDDEN_BY_DEFAULT = ['keyCount', 'binaryKeys', 'size'];
const CONFIGMAP_VIEW_COLUMNS = defaultViewColumns('configmaps', CONFIGMAP_HIDDEN_BY_DEFAULT);
const NO_DATA: any[] = [];

export default function ConfigMapList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { addNotification } = useNotification();
    const selection = useSelection();
    const [hideSystemConfigMaps, setHideSystemConfigMaps] = useState(true);

    const [bulkActionModal, setBulkActionModal] = useState<any>({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState<any>({ current: 0, total: 0, status: 'idle', results: [] });
    const pausedRef = useRef(false);
    const resumeResolverRef = useRef<(() => void) | null>(null);

    const handleBulkDeleteClick = useCallback((selectedItems: any) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handlePause = useCallback(() => {
        pausedRef.current = true;
        setBulkProgress((prev: any) => prev.status === 'inProgress' ? { ...prev, status: 'paused' } : prev);
    }, []);

    const handleResume = useCallback(() => {
        pausedRef.current = false;
        setBulkProgress((prev: any) => prev.status === 'paused' ? { ...prev, status: 'inProgress' } : prev);
        if (resumeResolverRef.current) {
            resumeResolverRef.current();
            resumeResolverRef.current = null;
        }
    }, []);

    const handleBulkActionConfirm = useCallback(async (items: any, delayMs: number = 0) => {
        pausedRef.current = false;
        resumeResolverRef.current = null;
        Logger.info('Bulk delete started', { count: items.length, delayMs }, 'config');
        setBulkProgress((prev: any) => ({ ...prev, status: 'inProgress', results: [] }));

        const results: any[] = [];
        for (let i = 0; i < items.length; i++) {
            if (pausedRef.current) {
                await new Promise<void>(resolve => { resumeResolverRef.current = resolve; });
            }
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

            // Delay between items, not after the last one
            if (delayMs > 0 && i < items.length - 1) {
                await sleep(delayMs);
            }
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
    // The backend view hides system ConfigMaps itself (preset), so the toggle is a view option.
    const viewOptions = useMemo(() => ({
        sort: CONFIGMAP_INITIAL_SORT,
        hide: hideSystemConfigMaps ? ['system'] : [],
        columns: CONFIGMAP_VIEW_COLUMNS,
    }), [hideSystemConfigMaps]);
    const { configMaps, loading, loadState, view } = useConfigMaps(currentContext, selectedNamespaces, isVisible, viewOptions) as any;
    const { handleEditYaml, handleEditKeyValue, handleShowDependencies, handleDelete } = useStableCallbacks(useConfigMapActions());

    // Filter out system ConfigMaps if toggle is enabled
    const filteredConfigMaps = useMemo(() => {
        if (!hideSystemConfigMaps) return configMaps;
        return configMaps.filter((cm: any) => !SYSTEM_CONFIGMAP_NAMES.includes(cm.metadata?.name));
    }, [configMaps, hideSystemConfigMaps]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: configMapSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: configMapSemantics.namespace.getValue },
        { key: 'keys', label: 'Keys', render: (item: any) => Object.keys(item.data || {}).join(', '), getValue: configMapSemantics.keys.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: configMapSemantics.age.getValue },
        // Hidden by default columns
        {
            key: 'keyCount',
            label: 'Key Count',
            defaultHidden: true,
            render: (item: any) => Object.keys(item.data || {}).length,
            getValue: configMapSemantics.keyCount.getValue,
        },
        {
            key: 'binaryKeys',
            label: 'Binary Keys',
            defaultHidden: true,
            render: (item: any) => {
                const count = Object.keys(item.binaryData || {}).length;
                return count > 0 ? count : <span className="text-gray-500">-</span>;
            },
            getValue: configMapSemantics.binaryKeys.getValue,
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
            getValue: configMapSemantics.size.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ConfigMapActionsMenu}
                    menuId={`configmap-${item.metadata.uid}`}
                    configMap={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, handleDelete]);

    const viewColumns = useMemo(
        () => buildConfigMapViewColumns({ onEditYaml: handleEditYaml, onShowDependencies: handleShowDependencies, onDelete: handleDelete }),
        [handleEditYaml, handleShowDependencies, handleDelete]);

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
            <ResourceList loadState={loadState}
                title="ConfigMaps"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : filteredConfigMaps}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={CONFIGMAP_INITIAL_SORT}
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
                onPause={handlePause}
                onResume={handleResume}
                onExportYaml={handleExportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
