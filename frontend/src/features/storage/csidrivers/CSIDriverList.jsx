import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import CSIDriverActionsMenu from './CSIDriverActionsMenu';
import { useCSIDrivers } from '../../../hooks/resources';
import { useCSIDriverActions } from './useCSIDriverActions';
import { useK8s } from '../../../context/K8sContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteCSIDriver, GetCSIDriverYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

const BooleanIcon = ({ value }) => {
    if (value === true) {
        return <CheckCircleIcon className="h-5 w-5 text-green-400" />;
    }
    return <XCircleIcon className="h-5 w-5 text-gray-500" />;
};

export default function CSIDriverList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useMenu();
    const { csiDrivers, loading } = useCSIDrivers(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleDelete } = useCSIDriverActions();
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
                await DeleteCSIDriver(name);
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
                const yaml = await GetCSIDriverYaml(item.metadata?.name);
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'CSIDriver', yaml });
            } catch (err) {
                entries.push({ namespace: '', name: item.metadata?.name, kind: 'CSIDriver', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `csidrivers-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
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

    const getAttachRequired = (driver) => {
        return driver.spec?.attachRequired ?? true;
    };

    const getPodInfoOnMount = (driver) => {
        return driver.spec?.podInfoOnMount ?? false;
    };

    const getStorageCapacity = (driver) => {
        return driver.spec?.storageCapacity ?? false;
    };

    const getVolumeModes = (driver) => {
        const modes = driver.spec?.volumeLifecycleModes || [];
        return modes.length > 0 ? modes.join(', ') : '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'attachRequired',
            label: 'Attach Required',
            render: (item) => <BooleanIcon value={getAttachRequired(item)} />,
            getValue: (item) => getAttachRequired(item) ? 'Yes' : 'No',
            align: 'center'
        },
        {
            key: 'podInfoOnMount',
            label: 'Pod Info on Mount',
            render: (item) => <BooleanIcon value={getPodInfoOnMount(item)} />,
            getValue: (item) => getPodInfoOnMount(item) ? 'Yes' : 'No',
            align: 'center'
        },
        {
            key: 'storageCapacity',
            label: 'Storage Capacity',
            render: (item) => <BooleanIcon value={getStorageCapacity(item)} />,
            getValue: (item) => getStorageCapacity(item) ? 'Yes' : 'No',
            align: 'center'
        },
        { key: 'volumeModes', label: 'Volume Modes', render: (item) => getVolumeModes(item), getValue: (item) => getVolumeModes(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <CSIDriverActionsMenu
                    csiDriver={item}
                    isOpen={activeMenuId === `csidriver-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `csidriver-${item.metadata.uid}`, buttonElement)}
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
                title="CSI Drivers"
                columns={columns}
                data={csiDrivers}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="csidrivers"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />
        </>
    );
}
