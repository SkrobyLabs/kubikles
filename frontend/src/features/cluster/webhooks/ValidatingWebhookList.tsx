import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import ValidatingWebhookActionsMenu from './ValidatingWebhookActionsMenu';
import { useValidatingWebhookConfigurations } from '~/hooks/resources';
import { useValidatingWebhookActions } from './useValidatingWebhookActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteValidatingWebhookConfiguration, GetValidatingWebhookConfigurationYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { validatingWebhookSemantics, getWebhookCount, getFailurePolicy } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildValidatingWebhookViewColumns } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const VALIDATINGWEBHOOK_INITIAL_SORT = { key: 'age', direction: 'desc' };
const VALIDATINGWEBHOOK_VIEW_OPTIONS = { sort: VALIDATINGWEBHOOK_INITIAL_SORT, columns: defaultViewColumns('validatingwebhookconfigurations') };
const NO_DATA: any[] = [];

export default function ValidatingWebhookList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { validatingWebhookConfigurations, loading, loadState, view } = useValidatingWebhookConfigurations(currentContext, isVisible, VALIDATINGWEBHOOK_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useStableCallbacks(useValidatingWebhookActions());
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
        resourceLabel: 'ValidatingWebhookConfiguration',
        resourceType: 'validatingwebhookconfigurations',
        isNamespaced: false,
        deleteApi: DeleteValidatingWebhookConfiguration,
        getYamlApi: GetValidatingWebhookConfigurationYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: validatingWebhookSemantics.name.getValue },
        { key: 'webhooks', label: 'Webhooks', render: (item: any) => getWebhookCount(item), getValue: validatingWebhookSemantics.webhooks.getValue },
        { key: 'failurePolicy', label: 'Failure Policy', render: (item: any) => getFailurePolicy(item), getValue: validatingWebhookSemantics.failurePolicy.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: validatingWebhookSemantics.age.getValue },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ValidatingWebhookActionsMenu}
                    menuId={`validatingwebhook-${item.metadata.uid}`}
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
        () => buildValidatingWebhookViewColumns({
            onEditYaml: handleEditYaml,
            onShowDependencies: handleShowDependencies,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Validating Webhook Configurations"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : validatingWebhookConfigurations}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={VALIDATINGWEBHOOK_INITIAL_SORT}
                resourceType="validatingwebhookconfigurations"
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
