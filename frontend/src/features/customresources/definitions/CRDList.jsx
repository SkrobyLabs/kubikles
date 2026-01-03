import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import { useCRDs } from '../../../hooks/useCRDs';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteCRD, GetCRDYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import CRDActionsMenu from './CRDActionsMenu';
import { useCRDActions } from './useCRDActions';
import Logger from '../../../utils/Logger';

export default function CRDList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { crds, loading } = useCRDs(currentContext, isVisible);
    const { handleEditYaml, handleDelete } = useCRDActions();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
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
            const name = item.metadata?.name;
            try {
                await DeleteCRD(name);
                results.push({ name, namespace: '', success: true, message: '' });
            } catch (err) {
                results.push({ name, namespace: '', success: false, message: err.toString() });
            }
            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }
        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, []);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        const entries = [];
        for (const item of items) {
            try {
                const yaml = await GetCRDYaml(item.metadata?.name);
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'CustomResourceDefinition', yaml });
            } catch (err) {
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'CustomResourceDefinition', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `crds-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
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

    // Get the served versions as a comma-separated string
    const getVersions = (crd) => {
        const versions = crd.spec?.versions || [];
        const served = versions.filter(v => v.served).map(v => v.name);
        return served.join(', ') || '-';
    };

    // Get the storage version (the one that is stored)
    const getStorageVersion = (crd) => {
        const versions = crd.spec?.versions || [];
        const storage = versions.find(v => v.storage);
        return storage?.name || '-';
    };

    const columns = useMemo(() => [
        {
            key: 'resource',
            label: 'Resource',
            render: (item) => item.spec?.names?.kind || '-',
            getValue: (item) => item.spec?.names?.kind || ''
        },
        {
            key: 'group',
            label: 'Group',
            render: (item) => item.spec?.group || '-',
            getValue: (item) => item.spec?.group || ''
        },
        {
            key: 'version',
            label: 'Version',
            render: (item) => {
                const versions = item.spec?.versions || [];
                const storageVersion = versions.find(v => v.storage);
                const servedVersions = versions.filter(v => v.served && !v.storage && !v.deprecated);
                const deprecatedVersions = versions.filter(v => v.served && v.deprecated);

                // Order: storage first, then served, then deprecated
                const orderedVersions = [
                    ...(storageVersion ? [storageVersion] : []),
                    ...servedVersions,
                    ...deprecatedVersions
                ];

                const maxDisplay = 3;
                const displayVersions = orderedVersions.slice(0, maxDisplay);
                const remaining = orderedVersions.length - maxDisplay;

                return (
                    <div className="flex flex-wrap gap-1 max-w-[200px]">
                        {displayVersions.map(v => (
                            <span
                                key={v.name}
                                className={`text-xs px-1.5 py-0.5 rounded ${
                                    v.storage
                                        ? 'bg-blue-500/20 text-blue-400'
                                        : v.deprecated
                                            ? 'bg-yellow-500/20 text-yellow-400'
                                            : 'bg-gray-500/20 text-gray-400'
                                }`}
                                title={v.storage ? 'Storage version' : v.deprecated ? 'Deprecated' : 'Served'}
                            >
                                {v.name}
                            </span>
                        ))}
                        {remaining > 0 && (
                            <span className="text-xs text-gray-500">+{remaining}</span>
                        )}
                    </div>
                );
            },
            getValue: (item) => getStorageVersion(item)
        },
        {
            key: 'scope',
            label: 'Scope',
            render: (item) => item.spec?.scope || '-',
            getValue: (item) => item.spec?.scope || ''
        },
        {
            key: 'age',
            label: 'Age',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <CRDActionsMenu
                    crd={item}
                    isOpen={activeMenuId === `crd-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `crd-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleDelete]);

    return (
        <>
            <ResourceList
                title="Custom Resource Definitions"
                columns={columns}
                data={crds}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="crds"
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />
        </>
    );
}
