import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import CSIDriverActionsMenu from './CSIDriverActionsMenu';
import { useCSIDrivers } from '../../../hooks/resources';
import { useCSIDriverActions } from './useCSIDriverActions';
import { useK8s } from '../../../context';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteCSIDriver, GetCSIDriverYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

const BooleanIcon = ({ value }) => {
    if (value === true) {
        return <CheckCircleIcon className="h-5 w-5 text-green-400" />;
    }
    return <XCircleIcon className="h-5 w-5 text-gray-500" />;
};

export default function CSIDriverList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { csiDrivers, loading } = useCSIDrivers(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml } = useCSIDriverActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'CSIDriver',
        resourceType: 'csidrivers',
        isNamespaced: false,
        deleteApi: DeleteCSIDriver,
        getYamlApi: GetCSIDriverYaml,
        currentContext,
    });

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
                    onDelete={(csiDriver) => openBulkDelete([csiDriver])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete]);

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
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
