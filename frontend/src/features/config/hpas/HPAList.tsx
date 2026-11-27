import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import HPAActionsMenu from './HPAActionsMenu';
import { useHPAs } from '~/hooks/resources';
import { useHPAActions } from './useHPAActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteHPA, GetHPAYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { hpaSemantics, hpaScaleTarget, hpaMinMax, hpaReplicas } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildHPAViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const HPA_INITIAL_SORT = { key: 'age', direction: 'desc' };
const HPA_VIEW_OPTIONS = { sort: HPA_INITIAL_SORT, columns: defaultViewColumns('hpas') };
const NO_DATA: any[] = [];

export default function HPAList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { hpas, loading, loadState, view } = useHPAs(currentContext, selectedNamespaces, isVisible, HPA_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useHPAActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'HorizontalPodAutoscaler',
        resourceType: 'hpas',
        isNamespaced: true,
        deleteApi: DeleteHPA,
        getYamlApi: GetHPAYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: hpaSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: hpaSemantics.namespace.getValue },
        { key: 'reference', label: 'Reference', render: (item: any) => hpaScaleTarget(item), getValue: hpaSemantics.reference.getValue },
        { key: 'minmax', label: 'Min/Max', render: (item: any) => hpaMinMax(item), getValue: hpaSemantics.minmax.getValue },
        { key: 'replicas', label: 'Replicas', render: (item: any) => hpaReplicas(item), getValue: hpaSemantics.replicas.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: hpaSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={HPAActionsMenu}
                    menuId={`hpa-${item.metadata.uid}`}
                    hpa={item}
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
        () => buildHPAViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Horizontal Pod Autoscalers"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : hpas}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={HPA_INITIAL_SORT}
                resourceType="hpas"
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
