import React from 'react';
import { PencilSquareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, StatusBadge, CopyableTextBlock, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor } from '../lazy';

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
    'PersistentVolumeClaim': 'pvcs',
    'PersistentVolume': 'pvs',
    'Ingress': 'ingresses',
};

export default function EventDetails({ event, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch } = useUI();

    // Check if this tab is stale
    const isStale = tabContext && tabContext !== currentContext;

    const type = event.type || 'Unknown';
    const reason = event.reason || 'Unknown';
    const message = event.message || '';
    const count = event.count || 1;
    const involvedObject = event.involvedObject || {};
    const source = event.source || {};

    const handleEditYaml = () => {
        const tabId = `yaml-event-${event.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${event.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="event"
                    namespace={event.metadata?.namespace}
                    resourceName={event.metadata?.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleNavigateToObject = () => {
        if (!involvedObject.kind || !involvedObject.uid) return;
        const viewName = kindToView[involvedObject.kind];
        if (viewName) {
            navigateWithSearch(viewName, `uid:"${involvedObject.uid}"`);
        }
    };

    const getTypeVariant = (type) => {
        switch (type) {
            case 'Normal': return 'success';
            case 'Warning': return 'warning';
            default: return 'default';
        }
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {event.metadata?.namespace}/{reason}
                    </div>
                    <StatusBadge status={type} variant={getTypeVariant(type)} />
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        <button
                            onClick={handleEditYaml}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Edit YAML"
                            disabled={isStale}
                        >
                            <PencilSquareIcon className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            <div className="h-full overflow-auto p-4">
                {/* Message - Prominent display */}
                <DetailSection title="Message">
                    <CopyableTextBlock value={message} maxLines={15} />
                </DetailSection>

                {/* Event Details */}
                <DetailSection title="Event Details">
                    <DetailRow label="Type">
                        <StatusBadge status={type} variant={getTypeVariant(type)} />
                    </DetailRow>
                    <DetailRow label="Reason" value={reason} />
                    <DetailRow label="Count" value={count.toString()} />
                    <DetailRow label="First Seen">
                        <span title={event.firstTimestamp}>
                            {formatAge(event.firstTimestamp || event.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="Last Seen">
                        <span title={event.lastTimestamp}>
                            {formatAge(event.lastTimestamp || event.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                </DetailSection>

                {/* Involved Object */}
                <DetailSection title="Involved Object">
                    <DetailRow label="Kind" value={involvedObject.kind} />
                    <DetailRow label="Name">
                        {kindToView[involvedObject.kind] ? (
                            <button
                                onClick={handleNavigateToObject}
                                className="text-primary hover:text-primary/80 hover:underline transition-colors"
                                title={`Go to ${involvedObject.kind}: ${involvedObject.name}`}
                            >
                                {involvedObject.name}
                            </button>
                        ) : (
                            <span>{involvedObject.name || 'N/A'}</span>
                        )}
                    </DetailRow>
                    <DetailRow label="Namespace" value={involvedObject.namespace} />
                    <DetailRow label="UID">
                        {involvedObject.uid ? (
                            <CopyableLabel value={involvedObject.uid.substring(0, 8) + '...'} copyValue={involvedObject.uid} />
                        ) : (
                            <span className="text-gray-500">N/A</span>
                        )}
                    </DetailRow>
                    <DetailRow label="API Version" value={involvedObject.apiVersion} />
                    <DetailRow label="Field Path" value={involvedObject.fieldPath} />
                </DetailSection>

                {/* Source */}
                <DetailSection title="Source">
                    <DetailRow label="Component" value={source.component} />
                    <DetailRow label="Host" value={source.host} />
                </DetailSection>

                {/* Metadata */}
                <DetailSection title="Metadata">
                    <DetailRow label="Name" value={event.metadata?.name} />
                    <DetailRow label="Namespace" value={event.metadata?.namespace} />
                    <DetailRow label="UID">
                        <CopyableLabel value={event.metadata?.uid?.substring(0, 8) + '...'} copyValue={event.metadata?.uid} />
                    </DetailRow>
                    <DetailRow label="Created">
                        <span title={event.metadata?.creationTimestamp}>
                            {formatAge(event.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                </DetailSection>
            </div>
        </div>
    );
}
