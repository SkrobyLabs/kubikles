import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import NetworkPolicyActionsMenu from './NetworkPolicyActionsMenu';
import { useNetworkPolicies } from '~/hooks/resources';
import { useNetworkPolicyActions } from './useNetworkPolicyActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteNetworkPolicy, GetNetworkPolicyYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function NetworkPolicyList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { networkPolicies, loading } = useNetworkPolicies(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useNetworkPolicyActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'NetworkPolicy',
        resourceType: 'networkpolicies',
        isNamespaced: true,
        deleteApi: DeleteNetworkPolicy,
        getYamlApi: GetNetworkPolicyYaml,

    });

    const getPodSelector = (policy: any) => {
        const selector = policy.spec?.podSelector;
        if (!selector || Object.keys(selector.matchLabels || {}).length === 0) {
            return 'All Pods';
        }
        const labels = Object.entries(selector.matchLabels || {})
            .map(([k, v]) => `${k}=${v}`)
            .slice(0, 2);
        const extra = Object.keys(selector.matchLabels || {}).length - 2;
        return labels.join(', ') + (extra > 0 ? `, +${extra}` : '');
    };

    const getPolicyTypes = (policy: any) => {
        return policy.spec?.policyTypes?.join(', ') || 'Ingress';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        { key: 'podSelector', label: 'Pod Selector', render: (item: any) => getPodSelector(item), getValue: (item: any) => getPodSelector(item) },
        { key: 'policyTypes', label: 'Policy Types', render: (item: any) => getPolicyTypes(item), getValue: (item: any) => getPolicyTypes(item) },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <NetworkPolicyActionsMenu
                    networkPolicy={item}
                    isOpen={activeMenuId === `networkpolicy-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `networkpolicy-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(policy: any) => openBulkDelete([policy])}
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
                title="Network Policies"
                columns={columns}
                data={networkPolicies}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="networkpolicies"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action || ''} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
