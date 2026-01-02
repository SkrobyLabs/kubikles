import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import ValidatingWebhookActionsMenu from './ValidatingWebhookActionsMenu';
import { useValidatingWebhookConfigurations } from '../../../hooks/resources';
import { useValidatingWebhookActions } from './useValidatingWebhookActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function ValidatingWebhookList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { validatingWebhookConfigurations, loading } = useValidatingWebhookConfigurations(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = useValidatingWebhookActions();
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

    const getWebhookCount = (config) => {
        return (config.webhooks || []).length;
    };

    const getFailurePolicy = (config) => {
        const policies = new Set();
        (config.webhooks || []).forEach(wh => {
            policies.add(wh.failurePolicy || 'Fail');
        });
        return Array.from(policies).join(', ') || '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'webhooks', label: 'Webhooks', render: (item) => getWebhookCount(item), getValue: (item) => getWebhookCount(item) },
        { key: 'failurePolicy', label: 'Failure Policy', render: (item) => getFailurePolicy(item), getValue: (item) => getFailurePolicy(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ValidatingWebhookActionsMenu
                    webhook={item}
                    isOpen={activeMenuId === `validatingwebhook-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `validatingwebhook-${item.metadata.uid}`, buttonElement)}
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
            title="Validating Webhook Configurations"
            columns={columns}
            data={validatingWebhookConfigurations}
            isLoading={loading}
            showNamespaceSelector={false}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="validatingwebhookconfigurations"
            onRowClick={handleShowDetails}
        />
    );
}
