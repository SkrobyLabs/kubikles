import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import LeaseActionsMenu from './LeaseActionsMenu';
import { useLeases } from '~/hooks/resources';
import { useLeaseActions } from './useLeaseActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteLease, GetLeaseYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { leaseSemantics, leaseHolderIdentity } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildLeaseViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const LEASE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const LEASE_VIEW_OPTIONS = { sort: LEASE_INITIAL_SORT, columns: defaultViewColumns('leases') };
const NO_DATA: any[] = [];

export default function LeaseList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { leases, loading, loadState, view } = useLeases(currentContext, selectedNamespaces, isVisible, LEASE_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useLeaseActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Lease',
        resourceType: 'leases',
        isNamespaced: true,
        deleteApi: DeleteLease,
        getYamlApi: GetLeaseYaml,

    });

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
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: leaseSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: leaseSemantics.namespace.getValue },
        { key: 'holder', label: 'Holder Identity', render: (item: any) => leaseHolderIdentity(item), getValue: leaseSemantics.holder.getValue },
        { key: 'duration', label: 'Duration', render: (item: any) => getLeaseDuration(item), getValue: leaseSemantics.duration.getValue },
        { key: 'renewTime', label: 'Last Renewed', render: (item: any) => getRenewTime(item), getValue: leaseSemantics.renewTime.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: leaseSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={LeaseActionsMenu}
                    menuId={`lease-${item.metadata.uid}`}
                    lease={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(item: any) => openBulkDelete([item])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildLeaseViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Leases"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : leases}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={LEASE_INITIAL_SORT}
                resourceType="leases"
                onRowClick={view ? showDetails : handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </>
    );
}
