import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import PDBActionsMenu from './PDBActionsMenu';
import { usePDBs } from '../../../hooks/resources';
import { usePDBActions } from './usePDBActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeletePDB, GetPDBYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

export default function PDBList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { pdbs, loading } = usePDBs(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = usePDBActions();
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
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;

            try {
                await DeletePDB(currentContext, namespace, name);
                results.push({ name, namespace, success: true, message: '' });
                Logger.info('PDB deleted', { namespace, name });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
                Logger.error('Failed to delete PDB', { namespace, name, error: err });
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
                const yaml = await GetPDBYaml(namespace, name);
                entries.push({ namespace, name, kind: 'PodDisruptionBudget', yaml });
            } catch (err) {
                entries.push({ namespace, name, kind: 'PodDisruptionBudget', yaml: `# Failed to fetch YAML: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try {
            await SaveYamlBackup(entries, `pdbs-backup-${timestamp}.zip`);
        } catch (err) {
            if (err && err.toString() !== '') alert('Failed to save backup: ' + err);
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

    const getMinAvailable = (pdb) => {
        return pdb.spec?.minAvailable ?? '-';
    };

    const getMaxUnavailable = (pdb) => {
        return pdb.spec?.maxUnavailable ?? '-';
    };

    const getAllowedDisruptions = (pdb) => {
        return pdb.status?.disruptionsAllowed ?? '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'minAvailable', label: 'Min Available', render: (item) => getMinAvailable(item), getValue: (item) => getMinAvailable(item) },
        { key: 'maxUnavailable', label: 'Max Unavailable', render: (item) => getMaxUnavailable(item), getValue: (item) => getMaxUnavailable(item) },
        { key: 'allowed', label: 'Allowed Disruptions', render: (item) => getAllowedDisruptions(item), getValue: (item) => getAllowedDisruptions(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PDBActionsMenu
                    pdb={item}
                    isOpen={activeMenuId === `pdb-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `pdb-${item.metadata.uid}`, buttonElement)}
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
                title="Pod Disruption Budgets"
                columns={columns}
                data={pdbs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="pdbs"
                onRowClick={handleShowDetails}
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
