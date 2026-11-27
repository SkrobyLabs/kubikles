import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useStorageClasses } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteStorageClass, GetStorageClassYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { storageClassSemantics, isDefaultStorageClass } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildStorageClassViewColumns, getReclaimPolicyColor, getBindingModeColor, STORAGE_CLASS_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const STORAGECLASS_INITIAL_SORT = { key: 'age', direction: 'desc' };
const STORAGECLASS_VIEW_OPTIONS = { sort: STORAGECLASS_INITIAL_SORT, columns: defaultViewColumns('storageclasses') };
const NO_DATA: any[] = [];
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import StorageClassActionsMenu from './StorageClassActionsMenu';
import { useStorageClassActions } from './useStorageClassActions';

export default function StorageClassList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { storageClasses, loading, loadState, view } = useStorageClasses(currentContext, isVisible, STORAGECLASS_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useStorageClassActions());
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
        resourceLabel: 'StorageClass',
        resourceType: 'storageclasses',
        isNamespaced: false,
        deleteApi: DeleteStorageClass,
        getYamlApi: GetStorageClassYaml,

    });

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item: any) => (
                <div className="flex items-center gap-2">
                    {item.metadata?.name}
                    {isDefaultStorageClass(item) && (
                        <span className="text-xs bg-blue-500/20 text-blue-400 px-1.5 py-0.5 rounded">default</span>
                    )}
                </div>
            ),
            getValue: storageClassSemantics.name.getValue
        },
        { key: 'provisioner', label: 'Provisioner', render: (item: any) => item.provisioner || '-', getValue: storageClassSemantics.provisioner.getValue },
        {
            key: 'reclaimPolicy',
            label: 'Reclaim Policy',
            render: (item: any) => item.reclaimPolicy ? (
                <span className={getReclaimPolicyColor(item.reclaimPolicy)}>{item.reclaimPolicy}</span>
            ) : '-',
            getValue: storageClassSemantics.reclaimPolicy.getValue
        },
        {
            key: 'volumeBindingMode',
            label: 'Volume Binding Mode',
            render: (item: any) => item.volumeBindingMode ? (
                <span className={getBindingModeColor(item.volumeBindingMode)}>{item.volumeBindingMode}</span>
            ) : '-',
            getValue: storageClassSemantics.volumeBindingMode.getValue
        },
        {
            key: 'allowVolumeExpansion',
            label: 'Allow Expansion',
            render: (item: any) => item.allowVolumeExpansion ? (
                <CheckCircleIcon className="h-5 w-5 text-green-400" />
            ) : (
                <span className="text-gray-500">-</span>
            ),
            getValue: storageClassSemantics.allowVolumeExpansion.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: storageClassSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={StorageClassActionsMenu}
                    menuId={`storageclass-${item.metadata.uid}`}
                    storageClass={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(storageClass: any) => openBulkDelete([storageClass])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildStorageClassViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Storage Classes"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={STORAGE_CLASS_PINNED_COLUMNS}
                data={view ? NO_DATA : storageClasses}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={STORAGECLASS_INITIAL_SORT}
                resourceType="storageclasses"
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
