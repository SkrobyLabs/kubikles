import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
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
import { useMenuPosition } from '~/hooks/useMenuPosition';

function getEventTypeColor(type: any) {
    switch (type) {
        case 'Normal':
            return 'text-green-400';
        case 'Warning':
            return 'text-yellow-400';
        default:
            return 'text-gray-400';
    }
}

export default function EventList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for involved object resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
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
    const { events, loading } = useEventsList(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml } = useEventActions();

    const columns = useMemo(() => [
        {
            key: 'type',
            label: 'Type',
            render: (item: any) => {
                const type = item.type || 'Unknown';
                return <span className={getEventTypeColor(type)}>{type}</span>;
            },
            getValue: (item: any) => item.type || ''
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (item: any) => item.metadata?.namespace,
            getValue: (item: any) => item.metadata?.namespace
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
            getValue: (item: any) => item.involvedObject ? `${item.involvedObject.kind}/${item.involvedObject.name}` : ''
        },
        {
            key: 'message',
            label: 'Message',
            render: (item: any) => item.message || '-',
            getValue: (item: any) => item.message || ''
        },
        {
            key: 'count',
            label: 'Count',
            render: (item: any) => item.count || 1,
            getValue: (item: any) => item.count || 1
        },
        {
            key: 'age',
            label: 'Age',
            render: (item: any) => formatAge(item.firstTimestamp || item.metadata?.creationTimestamp),
            getValue: (item: any) => item.firstTimestamp || item.metadata?.creationTimestamp
        },
        {
            key: 'last',
            label: 'Last',
            render: (item: any) => formatAge(item.lastTimestamp || item.metadata?.creationTimestamp),
            getValue: (item: any) => item.lastTimestamp || item.metadata?.creationTimestamp
        },
        // Hidden by default columns
        {
            key: 'reason',
            label: 'Reason',
            defaultHidden: true,
            render: (item: any) => item.reason || '-',
            getValue: (item: any) => item.reason || '',
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
            getValue: (item: any) => item.source?.component || item.reportingController || '',
        },
        {
            key: 'objectKind',
            label: 'Object Kind',
            defaultHidden: true,
            render: (item: any) => item.involvedObject?.kind || '-',
            getValue: (item: any) => item.involvedObject?.kind || '',
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <EventActionsMenu
                    event={item}
                    isOpen={activeMenuId === `event-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `event-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onDelete={() => openBulkDelete([item])}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete, navigateWithSearch, crds]);

    return (
        <>
            <ResourceList
                title="Events"
                columns={columns}
                data={events}
                isLoading={loading}
                showNamespaceSelector={true}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                multiSelectNamespaces={true}
                initialSort={{ key: 'last', direction: 'desc' }}
                resourceType="events"
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
