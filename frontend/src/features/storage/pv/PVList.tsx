import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { usePVs } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeletePV, GetPVYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import PVActionsMenu from './PVActionsMenu';
import { usePVActions } from './usePVActions';
import { pvSemantics } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildPVViewColumns, getStatusColor, getReclaimPolicyColor, renderAccessModes } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const PV_INITIAL_SORT = { key: 'age', direction: 'desc' };
const PV_VIEW_OPTIONS = { sort: PV_INITIAL_SORT, columns: defaultViewColumns('persistentvolumes') };
const NO_DATA: any[] = [];

export default function PVList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { pvs, loading, loadState, view } = usePVs(currentContext, isVisible, PV_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(usePVActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'PersistentVolume',
        resourceType: 'persistentvolumes',
        isNamespaced: false,
        deleteApi: DeletePV,
        getYamlApi: GetPVYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: pvSemantics.name.getValue },
        {
            key: 'capacity',
            label: 'Capacity',
            render: (item: any) => item.spec?.capacity?.storage || '-',
            getValue: pvSemantics.capacity.getValue
        },
        {
            key: 'accessModes',
            label: 'Access Modes',
            render: (item: any) => renderAccessModes(item.spec?.accessModes),
            getValue: pvSemantics.accessModes.getValue
        },
        {
            key: 'reclaimPolicy',
            label: 'Reclaim Policy',
            render: (item: any) => item.spec?.persistentVolumeReclaimPolicy ? (
                <span className={getReclaimPolicyColor(item.spec.persistentVolumeReclaimPolicy)}>
                    {item.spec.persistentVolumeReclaimPolicy}
                </span>
            ) : '-',
            getValue: pvSemantics.reclaimPolicy.getValue
        },
        {
            key: 'status',
            label: 'Status',
            render: (item: any) => (
                <span className={getStatusColor(item.status?.phase)}>
                    {item.status?.phase || 'Unknown'}
                </span>
            ),
            getValue: pvSemantics.status.getValue
        },
        {
            key: 'claim',
            label: 'Claim',
            render: (item: any) => item.spec?.claimRef ? `${item.spec.claimRef.namespace}/${item.spec.claimRef.name}` : '-',
            getValue: pvSemantics.claim.getValue
        },
        { key: 'storageClass', label: 'Storage Class', render: (item: any) => item.spec?.storageClassName || '-', getValue: pvSemantics.storageClass.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: pvSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={PVActionsMenu}
                    menuId={`pv-${item.metadata.uid}`}
                    pv={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(pv: any) => openBulkDelete([pv])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildPVViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Persistent Volumes"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : pvs}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={PV_INITIAL_SORT}
                resourceType="pvs"
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
