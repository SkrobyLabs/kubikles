import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import EventActionsMenu from './EventActionsMenu';
import { useEventsList } from '~/hooks/resources';
import { useEventActions } from './useEventActions';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useMenu } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteEvent, GetEventYAML } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { getOwnerViewId } from '~/utils/owner-navigation';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildEventViewColumns, getEventTypeColor } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';
import { eventSemantics } from './semantics';

const EVENT_INITIAL_SORT = { key: 'last', direction: 'desc' };
const EVENT_VIEW_OPTIONS = { sort: EVENT_INITIAL_SORT, columns: defaultViewColumns('events', ['reason', 'source', 'objectKind']) };
const NO_DATA: any[] = [];

export default function EventList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for involved object resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Event',
        resourceType: 'events',
        isNamespaced: true,
        deleteApi: DeleteEvent,
        getYamlApi: GetEventYAML,

    });
    const { events, loading, loadState, view } = useEventsList(currentContext, selectedNamespaces, isVisible, EVENT_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useEventActions());
    const withObject = useWithFullObject(view ?? null);
    // Details tabs read the whole event; rows of a windowed list fetch it at click time.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const columns = useMemo(() => [
        {
            key: 'type',
            label: 'Type',
            render: (item: any) => {
                const type = item.type || 'Unknown';
                return <span className={getEventTypeColor(type)}>{type}</span>;
            },
            getValue: eventSemantics.type.getValue
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (item: any) => item.metadata?.namespace,
            getValue: eventSemantics.namespace.getValue
        },
        {
            key: 'involvedObject',
            label: 'Involved Object',
            render: (item: any) => {
                const obj = item.involvedObject;
                if (!obj) {
                    return <span className="text-gray-600">-</span>;
                }

                const viewId = getOwnerViewId(obj, crds);
                const displayText = `${obj.kind}/${obj.name}`;

                if (viewId) {
                    return (
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                navigateWithSearch(viewId, `uid:"${obj.uid}"`);
                            }}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors truncate max-w-xs"
                            title={`Go to ${obj.kind}: ${obj.name}`}
                        >
                            {displayText}
                        </button>
                    );
                }

                return (
                    <span className="text-gray-400 truncate max-w-xs" title={obj.name}>
                        {displayText}
                    </span>
                );
            },
            getValue: eventSemantics.involvedObject.getValue
        },
        {
            key: 'message',
            label: 'Message',
            render: (item: any) => item.message || '-',
            getValue: eventSemantics.message.getValue
        },
        {
            key: 'count',
            label: 'Count',
            render: (item: any) => item.count || 1,
            getValue: eventSemantics.count.getValue
        },
        {
            key: 'age',
            label: 'Age',
            render: (item: any) => formatAge(item.firstTimestamp || item.metadata?.creationTimestamp),
            getValue: eventSemantics.age.getValue
        },
        {
            key: 'last',
            label: 'Last',
            render: (item: any) => formatAge(item.lastTimestamp || item.metadata?.creationTimestamp),
            getValue: eventSemantics.last.getValue
        },
        // Hidden by default columns
        {
            key: 'reason',
            label: 'Reason',
            defaultHidden: true,
            render: (item: any) => item.reason || '-',
            getValue: eventSemantics.reason.getValue,
        },
        {
            key: 'source',
            label: 'Source',
            defaultHidden: true,
            render: (item: any) => {
                const source = item.source || item.reportingController;
                if (!source) return <span className="text-gray-500">-</span>;
                const component = source.component || source;
                const host = source.host;
                return host ? `${component} (${host})` : component;
            },
            getValue: eventSemantics.source.getValue,
        },
        {
            key: 'objectKind',
            label: 'Object Kind',
            defaultHidden: true,
            render: (item: any) => item.involvedObject?.kind || '-',
            getValue: eventSemantics.objectKind.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={EventActionsMenu}
                    menuId={`event-${item.metadata.uid}`}
                    event={item}
                    onEditYaml={() => handleEditYaml(item)}
                    onDelete={() => openBulkDelete([item])}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleEditYaml, openBulkDelete, navigateWithSearch, crds]);

    const viewColumns = useMemo(
        () => buildEventViewColumns({
            view,
            crds,
            navigateWithSearch,
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [view, crds, navigateWithSearch, handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Events"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                data={view ? NO_DATA : events}
                isLoading={loading}
                showNamespaceSelector={true}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                multiSelectNamespaces={true}
                initialSort={EVENT_INITIAL_SORT}
                resourceType="events"
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
