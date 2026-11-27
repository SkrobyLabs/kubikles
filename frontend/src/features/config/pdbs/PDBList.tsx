import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import PDBActionsMenu from './PDBActionsMenu';
import { usePDBs } from '~/hooks/resources';
import { usePDBActions } from './usePDBActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeletePDB, GetPDBYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { pdbSemantics, pdbMinAvailable, pdbMaxUnavailable, pdbAllowedDisruptions } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildPDBViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const PDB_INITIAL_SORT = { key: 'age', direction: 'desc' };
const PDB_VIEW_OPTIONS = { sort: PDB_INITIAL_SORT, columns: defaultViewColumns('pdbs') };
const NO_DATA: any[] = [];

export default function PDBList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { pdbs, loading, loadState, view } = usePDBs(currentContext, selectedNamespaces, isVisible, PDB_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(usePDBActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'PodDisruptionBudget',
        resourceType: 'pdbs',
        isNamespaced: true,
        deleteApi: DeletePDB,
        getYamlApi: GetPDBYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: pdbSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: pdbSemantics.namespace.getValue },
        { key: 'minAvailable', label: 'Min Available', render: (item: any) => pdbMinAvailable(item), getValue: pdbSemantics.minAvailable.getValue },
        { key: 'maxUnavailable', label: 'Max Unavailable', render: (item: any) => pdbMaxUnavailable(item), getValue: pdbSemantics.maxUnavailable.getValue },
        { key: 'allowed', label: 'Allowed Disruptions', render: (item: any) => pdbAllowedDisruptions(item), getValue: pdbSemantics.allowed.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: pdbSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={PDBActionsMenu}
                    menuId={`pdb-${item.metadata.uid}`}
                    pdb={item}
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
        () => buildPDBViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Pod Disruption Budgets"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : pdbs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={PDB_INITIAL_SORT}
                resourceType="pdbs"
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
