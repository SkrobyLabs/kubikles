import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useServiceAccounts } from '~/hooks/resources';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteServiceAccount, GetServiceAccountYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ServiceAccountActionsMenu from './ServiceAccountActionsMenu';
import { useServiceAccountActions } from './useServiceAccountActions';
import { serviceAccountSemantics, serviceAccountSecretCount } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildServiceAccountViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const SERVICEACCOUNT_INITIAL_SORT = { key: 'age', direction: 'desc' };
const SERVICEACCOUNT_VIEW_OPTIONS = { sort: SERVICEACCOUNT_INITIAL_SORT, columns: defaultViewColumns('serviceaccounts') };
const NO_DATA: any[] = [];

export default function ServiceAccountList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { serviceAccounts, loading, loadState, view } = useServiceAccounts(currentContext, selectedNamespaces, isVisible, SERVICEACCOUNT_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useServiceAccountActions());
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
        resourceLabel: 'ServiceAccount',
        resourceType: 'serviceaccounts',
        isNamespaced: true,
        deleteApi: ((namespace: string, name: string) => DeleteServiceAccount(namespace, name)) as any,
        getYamlApi: GetServiceAccountYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: serviceAccountSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: serviceAccountSemantics.namespace.getValue },
        {
            key: 'secrets',
            label: 'Secrets',
            align: 'center',
            render: (item: any) => serviceAccountSecretCount(item),
            getValue: serviceAccountSemantics.secrets.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: serviceAccountSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ServiceAccountActionsMenu}
                    menuId={`serviceaccount-${item.metadata.uid}`}
                    serviceAccount={item}
                    onEditYaml={handleEditYaml}
                    onDelete={(serviceAccount: any) => openBulkDelete([serviceAccount])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildServiceAccountViewColumns({
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Service Accounts"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : serviceAccounts}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={SERVICEACCOUNT_INITIAL_SORT}
                resourceType="serviceaccounts"
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
