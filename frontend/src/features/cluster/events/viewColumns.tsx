import React from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import MenuBound from '~/components/shared/MenuBound';
import { formatAge } from '~/utils/formatting';
import { getOwnerViewId, kindToView } from '~/utils/owner-navigation';
import { defineViewColumns, type ViewColumn } from '~/lib/views/viewColumns';
import type { ViewHandle } from '~/lib/views/viewClient';
import type { RowItem } from '~/lib/views/rowItem';
import EventActionsMenu from './EventActionsMenu';

export function getEventTypeColor(type: any) {
    switch (type) {
        case 'Normal':
            return 'text-green-400';
        case 'Warning':
            return 'text-yellow-400';
        default:
            return 'text-gray-400';
    }
}

export interface EventViewColumnDeps {
    view: Pick<ViewHandle, 'getObject'> | null;
    crds: any[] | undefined;
    navigateWithSearch: (viewId: string, search: string) => void;
    onEditYaml: (item: any) => void;
    onDelete: (item: any) => void;
}

/**
 * The involved-object cell is the text "Kind/name". The link target (apiVersion, uid) lives on
 * the event object, so it is fetched when the link is clicked.
 */
export async function openInvolvedObject(deps: EventViewColumnDeps, row: RowItem): Promise<void> {
    if (!deps.view) return;
    try {
        const full: any = await deps.view.getObject(row);
        const obj = full?.involvedObject;
        const viewId = obj ? getOwnerViewId(obj, deps.crds ?? []) : null;
        if (viewId) deps.navigateWithSearch(viewId, `uid:"${obj.uid}"`);
    } catch {
        // The event is gone or unreachable: nothing to navigate to.
    }
}

/** Presentation of the windowed Event list. */
export function buildEventViewColumns(deps: EventViewColumnDeps): ViewColumn[] {
    const crdKinds = new Set<string>((deps.crds || []).map((c: any) => c.spec?.names?.kind).filter(Boolean));
    return defineViewColumns('events', [
        {
            key: 'type',
            label: 'Type',
            render: item => {
                const type = item.cells.type || 'Unknown';
                return <span className={getEventTypeColor(type)}>{type}</span>;
            },
        },
        { key: 'namespace', label: 'Namespace', render: item => item.metadata.namespace },
        {
            key: 'involvedObject',
            label: 'Involved Object',
            render: item => {
                const text = item.cells.involvedObject;
                if (!text) return <span className="text-gray-600">-</span>;
                const kind = text.slice(0, text.indexOf('/'));
                if (kindToView[kind] || crdKinds.has(kind)) {
                    return (
                        <button
                            onClick={(e) => { e.stopPropagation(); void openInvolvedObject(deps, item); }}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors truncate max-w-xs"
                            title={`Go to ${text}`}
                        >
                            {text}
                        </button>
                    );
                }
                return <span className="text-gray-400 truncate max-w-xs" title={text}>{text}</span>;
            },
        },
        { key: 'message', label: 'Message', render: item => item.cells.message || '-' },
        { key: 'count', label: 'Count', render: item => item.cells.count || 1 },
        { key: 'age', label: 'Age', render: item => formatAge(item.cells.age ?? undefined) },
        { key: 'last', label: 'Last', render: item => formatAge(item.cells.last ?? undefined) },
        { key: 'reason', label: 'Reason', defaultHidden: true, render: item => item.cells.reason || '-' },
        {
            key: 'source',
            label: 'Source',
            defaultHidden: true,
            render: item => item.cells.source || <span className="text-gray-500">-</span>,
        },
        { key: 'objectKind', label: 'Object Kind', defaultHidden: true, render: item => item.cells.objectKind || '-' },
    ], [{
        key: 'actions',
        label: <EllipsisVerticalIcon className="h-5 w-5" />,
        align: 'center',
        render: (item: any) => (
            <MenuBound
                as={EventActionsMenu}
                menuId={`event-${item.metadata.uid}`}
                event={item}
                onEditYaml={() => deps.onEditYaml(item)}
                onDelete={() => deps.onDelete(item)}
            />
        ),
        isColumnSelector: true,
        disableSort: true,
    }]);
}
