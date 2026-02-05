import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import ValidatingWebhookActionsMenu from './ValidatingWebhookActionsMenu';
import { useValidatingWebhookConfigurations } from '../../../hooks/resources';
import { useValidatingWebhookActions } from './useValidatingWebhookActions';
import { useK8s } from '../../../context';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteValidatingWebhookConfiguration, GetValidatingWebhookConfigurationYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function ValidatingWebhookList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { validatingWebhookConfigurations, loading } = useValidatingWebhookConfigurations(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useValidatingWebhookActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ValidatingWebhookConfiguration',
        resourceType: 'validatingwebhookconfigurations',
        isNamespaced: false,
        deleteApi: DeleteValidatingWebhookConfiguration,
        getYamlApi: GetValidatingWebhookConfigurationYaml,
        currentContext,
    });

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
                    onDelete={(webhook) => openBulkDelete([webhook])}
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
                title="Validating Webhook Configurations"
                columns={columns}
                data={validatingWebhookConfigurations}
                isLoading={loading}
                showNamespaceSelector={false}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="validatingwebhookconfigurations"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
