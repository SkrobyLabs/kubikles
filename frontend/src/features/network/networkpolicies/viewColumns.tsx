import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import NetworkPolicyActionsMenu from './NetworkPolicyActionsMenu';

export interface NetworkPolicyViewColumnDeps {
    onEditYaml: (item: any) => void;
    onShowDependencies: (item: any) => void;
    onDelete: (item: any) => void;
}

/** Presentation of the windowed NetworkPolicy list; data semantics come from the generated schema. */
export function buildNetworkPolicyViewColumns(deps: NetworkPolicyViewColumnDeps): ViewColumn[] {
    return defineViewColumns('networkpolicies', [
        { key: 'name', label: 'Name', render: item => item.metadata.name },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        { key: 'podSelector', label: 'Pod Selector', render: item => item.cells.podSelector },
        { key: 'policyTypes', label: 'Policy Types', render: item => item.cells.policyTypes },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={NetworkPolicyActionsMenu}
                menuId={`networkpolicy-${item.metadata.uid}`}
                networkPolicy={item}
                onEditYaml={deps.onEditYaml}
                onShowDependencies={deps.onShowDependencies}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
