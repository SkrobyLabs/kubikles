import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import NetworkPolicyActionsMenu from './NetworkPolicyActionsMenu';
import { useNetworkPolicies } from '../../../hooks/resources';
import { useNetworkPolicyActions } from './useNetworkPolicyActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function NetworkPolicyList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { networkPolicies, loading } = useNetworkPolicies(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = useNetworkPolicyActions();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            setMenuPosition({
                top: rect.bottom + 4,
                left: rect.right - 192
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId]);

    const getPodSelector = (policy) => {
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

    const getPolicyTypes = (policy) => {
        return policy.spec?.policyTypes?.join(', ') || 'Ingress';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'podSelector', label: 'Pod Selector', render: (item) => getPodSelector(item), getValue: (item) => getPodSelector(item) },
        { key: 'policyTypes', label: 'Policy Types', render: (item) => getPolicyTypes(item), getValue: (item) => getPolicyTypes(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <NetworkPolicyActionsMenu
                    networkPolicy={item}
                    isOpen={activeMenuId === `networkpolicy-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `networkpolicy-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleDelete]);

    return (
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
        />
    );
}
