import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import type { DynamicRowSchema } from '~/lib/views/protocol';
import type { ViewColumn } from '~/lib/views/viewColumns';
import CustomResourceActionsMenu from './CustomResourceActionsMenu';
import { renderColumnValue } from './renderColumnValue';

export interface CustomResourceViewColumnDeps {
    onShowDetails: (item: any) => void;
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
    operatorActions: any[];
    onOperatorAction: (action: any, item: any) => void;
}

const KIND_TO_PRINTER_TYPE: Record<string, string> = { time: 'date', bool: 'boolean', number: 'integer', string: 'string' };

/** Columns shown before the open result (and its schema) arrives, and when it carries none. */
const FALLBACK_SCHEMA_KEYS = ['name', 'age'];

/**
 * Presentation of a windowed custom resource list. The data columns come from the schema the
 * backend built from the CRD's printer columns; keys equal the legacy list's, so saved column
 * configuration keeps applying. defineViewColumns is not used because the schema is not known at
 * build time; the same rules (every schema column presented) hold by construction.
 */
export function buildCustomResourceViewColumns(
    schema: DynamicRowSchema | null | undefined,
    deps: CustomResourceViewColumnDeps,
): ViewColumn[] {
    const cols: ViewColumn[] = [];
    const schemaCols: DynamicRowSchema['columns'] = schema?.columns ?? FALLBACK_SCHEMA_KEYS.map(key => ({
        key, kind: key === 'age' ? 'time' as const : 'string' as const, filterType: 'none' as const,
        predefined: [], volatile: false, label: key === 'age' ? 'Age' : 'Name',
    }));
    for (const sc of schemaCols) {
        const key = sc.key;
        const filterable = sc.filterType !== 'none';
        let render: (item: any) => React.ReactNode;
        if (key === 'name') render = item => item.metadata?.name || '-';
        else if (key === 'namespace') render = item => item.metadata?.namespace || '-';
        else if (key === 'age') render = item => formatAge(item.cells?.age ?? undefined);
        else {
            const type = KIND_TO_PRINTER_TYPE[sc.kind] ?? 'string';
            render = item => renderColumnValue(item.cells?.[key], type);
        }
        cols.push({
            key,
            label: sc.label || key,
            render,
            getValue: (item: any) => item?.cells?.[key],
            defaultHidden: sc.hidden === true,
            filterable,
        });
    }
    cols.push({
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={CustomResourceActionsMenu}
                menuId={`cr-${item.metadata?.uid}`}
                resource={item}
                operatorActions={deps.operatorActions}
                onOperatorAction={deps.onOperatorAction}
                onShowDetails={deps.onShowDetails}
                onEditYaml={deps.onEditYaml}
                onDelete={deps.onDelete}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    });
    return cols;
}
