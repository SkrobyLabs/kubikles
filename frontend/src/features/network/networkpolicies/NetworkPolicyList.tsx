import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import NetworkPolicyActionsMenu from './NetworkPolicyActionsMenu';
import { useNetworkPolicies } from '~/hooks/resources';
import { useNetworkPolicyActions } from './useNetworkPolicyActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteNetworkPolicy, GetNetworkPolicyYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { networkPolicySemantics, networkPolicyPodSelector, networkPolicyTypes } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildNetworkPolicyViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const NETWORKPOLICY_INITIAL_SORT = { key: 'age', direction: 'desc' };
const NETWORKPOLICY_VIEW_OPTIONS = { sort: NETWORKPOLICY_INITIAL_SORT, columns: defaultViewColumns('networkpolicies') };
const NO_DATA: any[] = [];

export default function NetworkPolicyList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { networkPolicies, loading, loadState, view } = useNetworkPolicies(currentContext, selectedNamespaces, isVisible, NETWORKPOLICY_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useNetworkPolicyActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'NetworkPolicy',
        resourceType: 'networkpolicies',
        isNamespaced: true,
        deleteApi: DeleteNetworkPolicy,
        getYamlApi: GetNetworkPolicyYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: networkPolicySemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: networkPolicySemantics.namespace.getValue },
        { key: 'podSelector', label: 'Pod Selector', render: (item: any) => networkPolicyPodSelector(item), getValue: networkPolicySemantics.podSelector.getValue },
        { key: 'policyTypes', label: 'Policy Types', render: (item: any) => networkPolicyTypes(item), getValue: networkPolicySemantics.policyTypes.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: networkPolicySemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={NetworkPolicyActionsMenu}
                    menuId={`networkpolicy-${item.metadata.uid}`}
                    networkPolicy={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(policy: any) => openBulkDelete([policy])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildNetworkPolicyViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Network Policies"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : networkPolicies}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={NETWORKPOLICY_INITIAL_SORT}
                resourceType="networkpolicies"
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
