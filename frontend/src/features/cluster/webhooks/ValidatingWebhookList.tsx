import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import ValidatingWebhookActionsMenu from './ValidatingWebhookActionsMenu';
import { useValidatingWebhookConfigurations } from '~/hooks/resources';
import { useValidatingWebhookActions } from './useValidatingWebhookActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteValidatingWebhookConfiguration, GetValidatingWebhookConfigurationYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function ValidatingWebhookList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { validatingWebhookConfigurations, loading } = useValidatingWebhookConfigurations(currentContext, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useValidatingWebhookActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ValidatingWebhookConfiguration',
        resourceType: 'validatingwebhookconfigurations',
        isNamespaced: false,
        deleteApi: DeleteValidatingWebhookConfiguration,
        getYamlApi: GetValidatingWebhookConfigurationYaml,

    });

    const getWebhookCount = (config: any) => {
        return (config.webhooks || []).length;
    };

    const getFailurePolicy = (config: any) => {
        const policies = new Set<any>();
        (config.webhooks || []).forEach((wh: any) => {
            policies.add(wh.failurePolicy || 'Fail');
        });
        return Array.from(policies).join(', ') || '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'webhooks', label: 'Webhooks', render: (item: any) => getWebhookCount(item), getValue: (item: any) => getWebhookCount(item) },
        { key: 'failurePolicy', label: 'Failure Policy', render: (item: any) => getFailurePolicy(item), getValue: (item: any) => getFailurePolicy(item) },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <ValidatingWebhookActionsMenu
                    webhook={item}
                    isOpen={activeMenuId === `validatingwebhook-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `validatingwebhook-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(webhook: any) => openBulkDelete([webhook])}
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
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </>
    );
}
