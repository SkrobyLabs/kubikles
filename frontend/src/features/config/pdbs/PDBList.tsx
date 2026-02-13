import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import PDBActionsMenu from './PDBActionsMenu';
import { usePDBs } from '~/hooks/resources';
import { usePDBActions } from './usePDBActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeletePDB, GetPDBYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function PDBList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { pdbs, loading } = usePDBs(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = usePDBActions();
    const selection = useSelection();

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

    const getMinAvailable = (pdb: any) => {
        return pdb.spec?.minAvailable ?? '-';
    };

    const getMaxUnavailable = (pdb: any) => {
        return pdb.spec?.maxUnavailable ?? '-';
    };

    const getAllowedDisruptions = (pdb: any) => {
        return pdb.status?.disruptionsAllowed ?? '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        { key: 'minAvailable', label: 'Min Available', render: (item: any) => getMinAvailable(item), getValue: (item: any) => getMinAvailable(item) },
        { key: 'maxUnavailable', label: 'Max Unavailable', render: (item: any) => getMaxUnavailable(item), getValue: (item: any) => getMaxUnavailable(item) },
        { key: 'allowed', label: 'Allowed Disruptions', render: (item: any) => getAllowedDisruptions(item), getValue: (item: any) => getAllowedDisruptions(item) },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <PDBActionsMenu
                    pdb={item}
                    isOpen={activeMenuId === `pdb-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `pdb-${item.metadata.uid}`, buttonElement)}
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
                title="Pod Disruption Budgets"
                columns={columns}
                data={pdbs}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="pdbs"
                onRowClick={handleShowDetails}
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
