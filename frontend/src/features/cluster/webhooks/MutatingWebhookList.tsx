import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import MutatingWebhookActionsMenu from './MutatingWebhookActionsMenu';
import { useMutatingWebhookConfigurations } from '~/hooks/resources';
import { useMutatingWebhookActions } from './useMutatingWebhookActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteMutatingWebhookConfiguration, GetMutatingWebhookConfigurationYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function MutatingWebhookList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { mutatingWebhookConfigurations, loading } = useMutatingWebhookConfigurations(currentContext, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useMutatingWebhookActions();
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
        resourceLabel: 'MutatingWebhookConfiguration',
        resourceType: 'mutatingwebhookconfigurations',
        isNamespaced: false,
        deleteApi: DeleteMutatingWebhookConfiguration,
        getYamlApi: GetMutatingWebhookConfigurationYaml,

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

    const getReinvocationPolicy = (config: any) => {
        const policies = new Set<any>();
        (config.webhooks || []).forEach((wh: any) => {
            policies.add(wh.reinvocationPolicy || 'Never');
        });
        return Array.from(policies).join(', ') || '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'webhooks', label: 'Webhooks', render: (item: any) => getWebhookCount(item), getValue: (item: any) => getWebhookCount(item) },
        { key: 'failurePolicy', label: 'Failure Policy', render: (item: any) => getFailurePolicy(item), getValue: (item: any) => getFailurePolicy(item) },
        { key: 'reinvocation', label: 'Reinvocation', render: (item: any) => getReinvocationPolicy(item), getValue: (item: any) => getReinvocationPolicy(item) },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MutatingWebhookActionsMenu
                    webhook={item}
                    isOpen={activeMenuId === `mutatingwebhook-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `mutatingwebhook-${item.metadata.uid}`, buttonElement)}
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
                title="Mutating Webhook Configurations"
                columns={columns}
                data={mutatingWebhookConfigurations}
                isLoading={loading}
                showNamespaceSelector={false}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="mutatingwebhookconfigurations"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action || ''}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
