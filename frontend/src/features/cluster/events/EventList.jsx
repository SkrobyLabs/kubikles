import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import EventActionsMenu from './EventActionsMenu';
import { useEventsList } from '../../../hooks/resources';
import { useEventActions } from './useEventActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteEvent, GetEventYAML } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { getOwnerViewId } from '../../../utils/owner-navigation';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

function getEventTypeColor(type) {
    switch (type) {
        case 'Normal':
            return 'text-green-400';
        case 'Warning':
            return 'text-yellow-400';
        default:
            return 'text-gray-400';
    }
}

export default function EventList({ isVisible }) {
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
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Event',
        resourceType: 'events',
        isNamespaced: true,
        deleteApi: DeleteEvent,
        getYamlApi: GetEventYAML,
        currentContext,
    });
    const { events, loading } = useEventsList(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml } = useEventActions();

    const columns = useMemo(() => [
        {
            key: 'type',
            label: 'Type',
            render: (item) => {
                const type = item.type || 'Unknown';
                return <span className={getEventTypeColor(type)}>{type}</span>;
            },
            getValue: (item) => item.type || ''
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (item) => item.metadata?.namespace,
            getValue: (item) => item.metadata?.namespace
        },
        {
            key: 'involvedObject',
            label: 'Involved Object',
            render: (item) => {
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
            getValue: (item) => item.involvedObject ? `${item.involvedObject.kind}/${item.involvedObject.name}` : ''
        },
        {
            key: 'message',
            label: 'Message',
            render: (item) => item.message || '-',
            getValue: (item) => item.message || ''
        },
        {
            key: 'count',
            label: 'Count',
            render: (item) => item.count || 1,
            getValue: (item) => item.count || 1
        },
        {
            key: 'age',
            label: 'Age',
            render: (item) => formatAge(item.firstTimestamp || item.metadata?.creationTimestamp),
            getValue: (item) => item.firstTimestamp || item.metadata?.creationTimestamp
        },
        {
            key: 'last',
            label: 'Last',
            render: (item) => formatAge(item.lastTimestamp || item.metadata?.creationTimestamp),
            getValue: (item) => item.lastTimestamp || item.metadata?.creationTimestamp
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <EventActionsMenu
                    event={item}
                    isOpen={activeMenuId === `event-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `event-${item.metadata.uid}`, buttonElement)}
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
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
