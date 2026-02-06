import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import LeaseActionsMenu from './LeaseActionsMenu';
import { useLeases } from '~/hooks/resources';
import { useLeaseActions } from './useLeaseActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteLease, GetLeaseYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function LeaseList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { leases, loading } = useLeases(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useLeaseActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Lease',
        resourceType: 'leases',
        isNamespaced: true,
        deleteApi: DeleteLease,
        getYamlApi: GetLeaseYaml,

    });

    const getHolderIdentity = (lease: any) => {
        return lease.spec?.holderIdentity || '-';
    };

    const getLeaseDuration = (lease: any) => {
        const duration = lease.spec?.leaseDurationSeconds;
        if (!duration) return '-';
        return `${duration}s`;
    };

    const getRenewTime = (lease: any) => {
        const renewTime = lease.spec?.renewTime;
        if (!renewTime) return '-';
        return formatAge(renewTime);
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        { key: 'holder', label: 'Holder Identity', render: (item: any) => getHolderIdentity(item), getValue: (item: any) => getHolderIdentity(item) },
        { key: 'duration', label: 'Duration', render: (item: any) => getLeaseDuration(item), getValue: (item: any) => item.spec?.leaseDurationSeconds || 0 },
        { key: 'renewTime', label: 'Last Renewed', render: (item: any) => getRenewTime(item), getValue: (item: any) => item.spec?.renewTime || '' },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <LeaseActionsMenu
                    lease={item}
                    isOpen={activeMenuId === `lease-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `lease-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(item: any) => openBulkDelete([item])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList
                title="Leases"
                columns={columns}
                data={leases}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="leases"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action || ''} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
