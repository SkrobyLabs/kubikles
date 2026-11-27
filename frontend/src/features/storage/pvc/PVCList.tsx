import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { usePVCs } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeletePVC, GetPVCYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import PVCActionsMenu from './PVCActionsMenu';
import { usePVCActions } from './usePVCActions';
import { pvcSemantics, pvcCapacity } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildPVCViewColumns, PVC_HIDDEN_BY_DEFAULT, getStatusColor, renderAccessModes } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const PVC_INITIAL_SORT = { key: 'age', direction: 'desc' };
const PVC_VIEW_OPTIONS = { sort: PVC_INITIAL_SORT, columns: defaultViewColumns('persistentvolumeclaims', PVC_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function PVCList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { pvcs, loading, loadState, view } = usePVCs(currentContext, selectedNamespaces, isVisible, PVC_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(usePVCActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'PersistentVolumeClaim',
        resourceType: 'pvcs',
        isNamespaced: true,
        deleteApi: DeletePVC,
        getYamlApi: GetPVCYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: pvcSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: pvcSemantics.namespace.getValue },
        {
            key: 'status',
            label: 'Status',
            render: (item: any) => (
                <span className={getStatusColor(item.status?.phase)}>
                    {item.status?.phase || 'Unknown'}
                </span>
            ),
            getValue: pvcSemantics.status.getValue
        },
        { key: 'volume', label: 'Volume', render: (item: any) => item.spec?.volumeName || '-', getValue: pvcSemantics.volume.getValue },
        {
            key: 'capacity',
            label: 'Capacity',
            render: (item: any) => pvcCapacity(item) || '-',
            getValue: pvcSemantics.capacity.getValue
        },
        { key: 'accessModes', label: 'Access Modes', render: (item: any) => renderAccessModes(item.spec?.accessModes), getValue: pvcSemantics.accessModes.getValue },
        { key: 'storageClass', label: 'Storage Class', render: (item: any) => item.spec?.storageClassName || '-', getValue: pvcSemantics.storageClass.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: pvcSemantics.age.getValue },
        // Hidden by default columns
        {
            key: 'volumeMode',
            label: 'Volume Mode',
            defaultHidden: true,
            render: (item: any) => item.spec?.volumeMode || 'Filesystem',
            getValue: pvcSemantics.volumeMode.getValue,
        },
        {
            key: 'requested',
            label: 'Requested',
            defaultHidden: true,
            render: (item: any) => item.spec?.resources?.requests?.storage || '-',
            getValue: pvcSemantics.requested.getValue,
        },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: (item: any) => {
                const labels = item.spec?.selector?.matchLabels || {};
                const entries = Object.entries(labels);
                if (entries.length === 0) return <span className="text-gray-500">-</span>;
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: pvcSemantics.selector.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={PVCActionsMenu}
                    menuId={`pvc-${item.metadata.uid}`}
                    pvc={item}
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
        () => buildPVCViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Persistent Volume Claims"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : pvcs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={PVC_INITIAL_SORT}
                resourceType="pvcs"
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
