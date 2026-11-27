import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import ReplicaSetActionsMenu from './ReplicaSetActionsMenu';
import { useReplicaSets } from '~/hooks/resources';
import { useReplicaSetActions } from './useReplicaSetActions';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useMenu } from '~/context';
import { useNotification } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteReplicaSet, GetReplicaSetYaml, ScaleReplicaSet } from 'wailsjs/go/main/App';
import ScaleModal from '~/components/shared/ScaleModal';
import { formatAge } from '~/utils/formatting';

import { replicaSetSemantics } from './semantics';
import { getController } from '../workloadSemantics';
import { renderControlledBy } from '../ControlledBy';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildReplicaSetViewColumns, REPLICASET_HIDDEN_BY_DEFAULT, REPLICASET_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const REPLICASET_INITIAL_SORT = { key: 'age', direction: 'desc' };
const REPLICASET_VIEW_OPTIONS = { sort: REPLICASET_INITIAL_SORT, columns: defaultViewColumns('replicasets', REPLICASET_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function ReplicaSetList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for owner reference resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const selection = useSelection();

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ReplicaSet',
        resourceType: 'replicasets',
        isNamespaced: true,
        deleteApi: DeleteReplicaSet,
        getYamlApi: GetReplicaSetYaml,

    });
    const { replicaSets, loading, loadState, view } = useReplicaSets(currentContext, selectedNamespaces, isVisible, REPLICASET_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useStableCallbacks(useReplicaSetActions());
    const { addNotification } = useNotification();
    const [scaleTarget, setScaleTarget] = useState<any>(null);
    const withObject = useWithFullObject(view ?? null);
    // Details and scaling need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);
    const startScale = useMemo(() => withObject(setScaleTarget), [withObject]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: replicaSetSemantics.name.getValue, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: replicaSetSemantics.namespace.getValue },
        {
            key: 'desired',
            label: 'Desired',
            render: (item: any) => item.spec?.replicas || 0,
            getValue: replicaSetSemantics.desired.getValue
        },
        {
            key: 'current',
            label: 'Current',
            render: (item: any) => item.status?.replicas || 0,
            getValue: replicaSetSemantics.current.getValue
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item: any) => item.status?.readyReplicas || 0,
            getValue: replicaSetSemantics.ready.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: replicaSetSemantics.age.getValue },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item: any) => renderControlledBy(getController(item), crds, navigateWithSearch),
            getValue: replicaSetSemantics.controlledBy.getValue
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ReplicaSetActionsMenu}
                    menuId={`rs-${item.metadata.uid}`}
                    replicaSet={item}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
                    onScale={() => setScaleTarget(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleEditYaml, handleShowDependencies, openBulkDelete, handleViewLogs, navigateWithSearch, crds]);

    const viewColumns = useMemo(
        () => buildReplicaSetViewColumns({
            crds,
            navigateWithSearch,
            onEditYaml: handleEditYaml as (item: any) => void,
            onShowDependencies: handleShowDependencies as (item: any) => void,
            onDelete: item => openBulkDelete([item]),
            onViewLogs: handleViewLogs as (item: any) => void,
            onScale: startScale,
        }),
        [crds, navigateWithSearch, handleEditYaml, handleShowDependencies, openBulkDelete, handleViewLogs, startScale]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="ReplicaSets"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={REPLICASET_PINNED_COLUMNS}
                data={view ? NO_DATA : replicaSets}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={REPLICASET_INITIAL_SORT}
                resourceType="replicasets"
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

            {scaleTarget && (
                <ScaleModal
                    resourceType="ReplicaSet"
                    resourceName={scaleTarget.metadata?.name || ''}
                    namespace={scaleTarget.metadata?.namespace || ''}
                    currentReplicas={scaleTarget.spec?.replicas ?? 1}
                    selector={scaleTarget.spec?.selector?.matchLabels || {}}
                    onScale={async (replicas: number) => {
                        await ScaleReplicaSet(scaleTarget.metadata?.namespace || '', scaleTarget.metadata?.name || '', replicas);
                        addNotification({ type: 'success', message: `Scaled ${scaleTarget.metadata?.name} to ${replicas} replicas` });
                    }}
                    onClose={() => setScaleTarget(null)}
                />
            )}
        </>
    );
}
