import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import EventActionsMenu from './EventActionsMenu';
import { useEventsList } from '../../../hooks/resources';
import { useEventActions } from './useEventActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteEvent, GetEventYAML, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

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
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    const [bulkActionModal, setBulkActionModal] = useState({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState({ current: 0, total: 0, status: 'idle', results: [] });

    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkActionConfirm = useCallback(async (items) => {
        Logger.info('Bulk delete started', { count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));
        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;
            try {
                await DeleteEvent(namespace, name);
                results.push({ name, namespace, success: true, message: '' });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
            }
            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }
        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, []);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        const entries = [];
        for (const item of items) {
            try {
                const yaml = await GetEventYAML(item.metadata?.namespace, item.metadata?.name);
                entries.push({ namespace: item.metadata?.namespace, name: item.metadata?.name, kind: 'Event', yaml });
            } catch (err) {
                entries.push({ namespace: item.metadata?.namespace, name: item.metadata?.name, kind: 'Event', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `events-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
    }, []);

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            setMenuPosition({
                top: rect.bottom + 4,
                left: rect.right - 192
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId]);
    const { events, loading } = useEventsList(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleDelete } = useEventActions();

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
                    onDelete={() => handleDelete(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleDelete, navigateWithSearch]);

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
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />
        </>
    );
}
