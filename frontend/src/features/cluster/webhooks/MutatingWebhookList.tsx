import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import MutatingWebhookActionsMenu from './MutatingWebhookActionsMenu';
import { useMutatingWebhookConfigurations } from '~/hooks/resources';
import { useMutatingWebhookActions } from './useMutatingWebhookActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteMutatingWebhookConfiguration, GetMutatingWebhookConfigurationYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { mutatingWebhookSemantics, getWebhookCount, getFailurePolicy, getReinvocationPolicy } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildMutatingWebhookViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const MUTATINGWEBHOOK_INITIAL_SORT = { key: 'age', direction: 'desc' };
const MUTATINGWEBHOOK_VIEW_OPTIONS = { sort: MUTATINGWEBHOOK_INITIAL_SORT, columns: defaultViewColumns('mutatingwebhookconfigurations') };
const NO_DATA: any[] = [];

export default function MutatingWebhookList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { mutatingWebhookConfigurations, loading, loadState, view } = useMutatingWebhookConfigurations(currentContext, isVisible, MUTATINGWEBHOOK_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useMutatingWebhookActions());
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
        resourceLabel: 'MutatingWebhookConfiguration',
        resourceType: 'mutatingwebhookconfigurations',
        isNamespaced: false,
        deleteApi: DeleteMutatingWebhookConfiguration,
        getYamlApi: GetMutatingWebhookConfigurationYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: mutatingWebhookSemantics.name.getValue },
        { key: 'webhooks', label: 'Webhooks', render: (item: any) => getWebhookCount(item), getValue: mutatingWebhookSemantics.webhooks.getValue },
        { key: 'failurePolicy', label: 'Failure Policy', render: (item: any) => getFailurePolicy(item), getValue: mutatingWebhookSemantics.failurePolicy.getValue },
        { key: 'reinvocation', label: 'Reinvocation', render: (item: any) => getReinvocationPolicy(item), getValue: mutatingWebhookSemantics.reinvocation.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: mutatingWebhookSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={MutatingWebhookActionsMenu}
                    menuId={`mutatingwebhook-${item.metadata.uid}`}
                    webhook={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(webhook: any) => openBulkDelete([webhook])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildMutatingWebhookViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Mutating Webhook Configurations"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : mutatingWebhookConfigurations}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={MUTATINGWEBHOOK_INITIAL_SORT}
                resourceType="mutatingwebhookconfigurations"
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
