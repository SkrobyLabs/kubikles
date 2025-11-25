import React, { useMemo } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import EventActionsMenu from './EventActionsMenu';
import { useEventsList } from '../../../hooks/useEventsList';
import { useEventActions } from './useEventActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

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

// Map Kubernetes resource kinds to view names
const kindToView = {
    'Pod': 'pods',
    'Deployment': 'deployments',
    'ReplicaSet': 'replicasets',
    'StatefulSet': 'statefulsets',
    'DaemonSet': 'daemonsets',
    'Job': 'jobs',
    'CronJob': 'cronjobs',
    'Service': 'services',
    'ConfigMap': 'configmaps',
    'Secret': 'secrets',
    'Node': 'nodes',
    'Namespace': 'namespaces',
};

export default function EventList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId, navigateWithSearch } = useUI();
    const { events, loading } = useEventsList(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleDelete } = useEventActions();

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

                const viewName = kindToView[obj.kind];
                const displayText = `${obj.kind}/${obj.name}`;

                if (viewName) {
                    return (
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                navigateWithSearch(viewName, `uid:"${obj.uid}"`);
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
            render: (item) => (
                <span className="truncate max-w-md block" title={item.message}>
                    {item.message || '-'}
                </span>
            ),
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
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `event-${item.metadata.uid}` : null)}
                    onEditYaml={() => handleEditYaml(item)}
                    onDelete={() => handleDelete(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleDelete, navigateWithSearch]);

    return (
        <ResourceList
            title="Events"
            columns={columns}
            data={events}
            isLoading={loading}
            showNamespaceSelector={true}
            namespaces={namespaces}
            selectedNamespaces={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            initialSort={{ key: 'last', direction: 'desc' }}
            resourceType="events"
        />
    );
}
