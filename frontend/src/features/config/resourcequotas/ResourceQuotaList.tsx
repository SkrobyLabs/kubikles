import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import ResourceQuotaActionsMenu from './ResourceQuotaActionsMenu';
import { useResourceQuotas } from '~/hooks/resources';
import { useResourceQuotaActions } from './useResourceQuotaActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteResourceQuota, GetResourceQuotaYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { resourceQuotaSemantics, resourceQuotaCount } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildResourceQuotaViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const RESOURCEQUOTA_INITIAL_SORT = { key: 'age', direction: 'desc' };
const RESOURCEQUOTA_VIEW_OPTIONS = { sort: RESOURCEQUOTA_INITIAL_SORT, columns: defaultViewColumns('resourcequotas') };
const NO_DATA: any[] = [];

export default function ResourceQuotaList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { resourceQuotas, loading, loadState, view } = useResourceQuotas(currentContext, selectedNamespaces, isVisible, RESOURCEQUOTA_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useResourceQuotaActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ResourceQuota',
        resourceType: 'resourcequotas',
        isNamespaced: true,
        deleteApi: DeleteResourceQuota,
        getYamlApi: GetResourceQuotaYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: resourceQuotaSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: resourceQuotaSemantics.namespace.getValue },
        { key: 'resources', label: 'Resources', render: (item: any) => resourceQuotaCount(item), getValue: resourceQuotaSemantics.resources.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: resourceQuotaSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ResourceQuotaActionsMenu}
                    menuId={`resourcequota-${item.metadata.uid}`}
                    resourceQuota={item}
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
        () => buildResourceQuotaViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Resource Quotas"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : resourceQuotas}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={RESOURCEQUOTA_INITIAL_SORT}
                resourceType="resourcequotas"
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
