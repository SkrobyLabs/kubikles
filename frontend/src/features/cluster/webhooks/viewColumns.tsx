import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import ValidatingWebhookActionsMenu from './ValidatingWebhookActionsMenu';
import MutatingWebhookActionsMenu from './MutatingWebhookActionsMenu';

export interface WebhookViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed ValidatingWebhookConfiguration list. */
export function buildValidatingWebhookViewColumns(deps: WebhookViewColumnDeps): ViewColumn[] {
    return defineViewColumns('validatingwebhookconfigurations', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'webhooks', label: 'Webhooks', render: item => item.cells.webhooks },
        { key: 'failurePolicy', label: 'Failure Policy', render: item => item.cells.failurePolicy },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={ValidatingWebhookActionsMenu}
                menuId={`validatingwebhook-${item.metadata.uid}`}
                webhook={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}

/** Presentation of the windowed MutatingWebhookConfiguration list. */
export function buildMutatingWebhookViewColumns(deps: WebhookViewColumnDeps): ViewColumn[] {
    return defineViewColumns('mutatingwebhookconfigurations', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'webhooks', label: 'Webhooks', render: item => item.cells.webhooks },
        { key: 'failurePolicy', label: 'Failure Policy', render: item => item.cells.failurePolicy },
        { key: 'reinvocation', label: 'Reinvocation', render: item => item.cells.reinvocation },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={MutatingWebhookActionsMenu}
                menuId={`mutatingwebhook-${item.metadata.uid}`}
                webhook={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
