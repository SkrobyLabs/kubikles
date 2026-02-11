import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { PencilSquareIcon, ShareIcon, LockClosedIcon } from '@heroicons/react/24/outline';
import { ExclamationTriangleIcon, InformationCircleIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';
import { GetCustomResourceYaml, UpdateCustomResourceYaml, GetCustomResourceEvents } from 'wailsjs/go/main/App';
import { useCRDWatcher } from '~/hooks/useResourceWatcher';

const TAB_BASIC = 'basic';
const TAB_EVENTS = 'events';

// Event type indicator
const EventTypeIcon = ({ type }: { type: string }) => {
    if (type === 'Warning') {
        return <ExclamationTriangleIcon className="w-4 h-4 text-yellow-400" />;
    }
    return <InformationCircleIcon className="w-4 h-4 text-green-400" />;
};

interface CRDInfo {
    group: string;
    version: string;
    resource: string;
    kind: string;
    namespaced: boolean;
}

interface CustomResourceDetailsProps {
    resource: any;
    crdInfo: CRDInfo;
    tabContext?: string;
}

export default function CustomResourceDetails({ resource: initialResource, crdInfo, tabContext = '' }: CustomResourceDetailsProps) {
    const { currentContext, lastRefresh } = useK8s();
    const { openTab, closeTab, getDetailTab, setDetailTab } = useUI();
    const activeTab = getDetailTab('customresource', TAB_BASIC);
    const setActiveTab = (tab: string) => setDetailTab('customresource', tab);

    // Track the current resource state, updating from watcher
    const [resource, setResource] = useState(initialResource);
    const [events, setEvents] = useState<any[]>([]);
    const [eventsLoading, setEventsLoading] = useState(false);

    const isStale = tabContext && tabContext !== currentContext;

    const name = resource.metadata?.name;
    const namespace = resource.metadata?.namespace || '';
    const uid = resource.metadata?.uid;
    const labels = resource.metadata?.labels || {};
    const annotations = resource.metadata?.annotations || {};
    const ownerReferences = resource.metadata?.ownerReferences || [];
    const status = resource.status || {};
    const conditions: any[] = status.conditions || [];

    // Subscribe to CRD watcher for real-time updates to this specific resource
    const handleWatcherEvent = useCallback((event: any) => {
        if (event.resource?.metadata?.uid === uid) {
            if (event.type === 'MODIFIED' || event.type === 'ADDED') {
                setResource(event.resource);
            }
        }
    }, [uid]);

    useCRDWatcher(
        crdInfo.group,
        crdInfo.version,
        crdInfo.resource,
        namespace ? [namespace] : [''],
        handleWatcherEvent,
        Boolean(!isStale)
    );

    // Fetch events when Events tab is active
    useEffect(() => {
        if (activeTab !== TAB_EVENTS || isStale || !name) return;

        const fetchEvents = async () => {
            setEventsLoading(true);
            try {
                const list = await GetCustomResourceEvents(
                    crdInfo.group,
                    crdInfo.version,
                    crdInfo.resource,
                    namespace,
                    name,
                    crdInfo.kind
                );
                setEvents(list || []);
            } catch (err: any) {
                console.error('Failed to fetch custom resource events:', err);
            } finally {
                setEventsLoading(false);
            }
        };

        fetchEvents();
    }, [activeTab, currentContext, namespace, name, crdInfo.group, crdInfo.version, crdInfo.resource, crdInfo.kind, isStale, lastRefresh]);

    // Sort events by last timestamp (most recent first)
    const sortedEvents = useMemo(() => {
        return [...events].sort((a, b) => {
            const timeA = new Date(a.lastTimestamp || a.metadata?.creationTimestamp || 0).getTime();
            const timeB = new Date(b.lastTimestamp || b.metadata?.creationTimestamp || 0).getTime();
            return timeB - timeA;
        });
    }, [events]);

    const handleEditYaml = () => {
        const tabId = `cr-yaml-${crdInfo.group}-${crdInfo.resource}-${namespace}-${name}`;
        openTab({
            id: tabId,
            title: `${name} (${crdInfo.kind})`,
            content: (
                <YamlEditor
                    resourceType="customresource"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    getYamlFn={() => GetCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, namespace, name)}
                    updateYamlFn={(content: string) => UpdateCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, namespace, name, content)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: crdInfo.kind, name, namespace: namespace || undefined },
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-cr-${crdInfo.group}-${crdInfo.resource}-${namespace}-${name}`;
        openTab({
            id: tabId,
            title: `${name} (${crdInfo.kind})`,
            content: (
                <DependencyGraph
                    resourceType="customresource"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            ),
            resourceMeta: { kind: crdInfo.kind, name, namespace: namespace || undefined },
        });
    };

    const getConditionVariant = (condition: any) => {
        if (condition.status === 'True') return 'success';
        if (condition.status === 'False') return 'error';
        return 'warning';
    };

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_EVENTS, label: 'Events' },
    ], []);

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <LockClosedIcon className="h-5 w-5" />
                    <span className="text-sm">
                        This resource is from context <span className="font-medium">{tabContext}</span>.
                    </span>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {namespace ? `${namespace}/` : ''}{name}
                    </div>
                    <StatusBadge status={crdInfo.kind} variant="info" />
                    {/* Tab Toggle */}
                    <div className="flex items-center bg-surface-light rounded-md p-0.5">
                        {tabs.map((tab) => (
                            <button
                                key={tab.id}
                                onClick={() => setActiveTab(tab.id)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    activeTab === tab.id
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                {tab.label}
                            </button>
                        ))}
                    </div>
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        <button
                            onClick={handleEditYaml}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Edit YAML"
                            disabled={!!isStale}
                        >
                            <PencilSquareIcon className="w-4 h-4" />
                        </button>
                        <button
                            onClick={handleShowDependencies}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Dependencies"
                        >
                            <ShareIcon className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            {activeTab === TAB_EVENTS ? (
                <div className="flex flex-col h-full overflow-hidden">
                    {eventsLoading && events.length === 0 ? (
                        <div className="flex items-center justify-center h-full text-gray-400">
                            Loading events...
                        </div>
                    ) : sortedEvents.length === 0 ? (
                        <div className="flex items-center justify-center h-full text-gray-500">
                            No events found for this resource
                        </div>
                    ) : (
                        <div className="flex-1 overflow-auto">
                            <table className="w-full text-sm">
                                <thead className="sticky top-0 bg-surface border-b border-border">
                                    <tr className="text-left text-xs text-gray-500 uppercase tracking-wider">
                                        <th className="px-4 py-2 w-10"></th>
                                        <th className="px-4 py-2">Reason</th>
                                        <th className="px-4 py-2">Message</th>
                                        <th className="px-4 py-2 w-16 text-center">Count</th>
                                        <th className="px-4 py-2 w-24">First</th>
                                        <th className="px-4 py-2 w-24">Last</th>
                                    </tr>
                                </thead>
                                <tbody>
                                    {sortedEvents.map((event) => (
                                        <tr
                                            key={event.metadata?.uid}
                                            className="border-b border-border/50 hover:bg-white/5"
                                        >
                                            <td className="px-4 py-2">
                                                <EventTypeIcon type={event.type} />
                                            </td>
                                            <td className="px-4 py-2 font-medium text-gray-200">
                                                {event.reason || '-'}
                                            </td>
                                            <td className="px-4 py-2 text-gray-400">
                                                <span className="line-clamp-2" title={event.message}>
                                                    {event.message || '-'}
                                                </span>
                                            </td>
                                            <td className="px-4 py-2 text-center text-gray-400">
                                                {event.count || 1}
                                            </td>
                                            <td className="px-4 py-2 text-gray-500 text-xs">
                                                {formatAge(event.firstTimestamp || event.metadata?.creationTimestamp)}
                                            </td>
                                            <td className="px-4 py-2 text-gray-500 text-xs">
                                                {formatAge(event.lastTimestamp || event.metadata?.creationTimestamp)}
                                            </td>
                                        </tr>
                                    ))}
                                </tbody>
                            </table>
                        </div>
                    )}
                </div>
            ) : (
                <div className="h-full overflow-auto p-4">
                    {/* Status Conditions */}
                    {conditions.length > 0 && (
                        <DetailSection title="Status Conditions">
                            <div className="space-y-2">
                                {conditions.map((condition: any, idx: number) => (
                                    <div key={idx} className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-0">
                                        <div className="flex items-center gap-2">
                                            <StatusBadge status={condition.type} variant={getConditionVariant(condition)} />
                                            {condition.reason && (
                                                <span className="text-xs text-gray-500">({condition.reason})</span>
                                            )}
                                            <span className="text-sm text-gray-400">{condition.message}</span>
                                        </div>
                                        <span className="text-xs text-gray-500 shrink-0 ml-2" title={condition.lastTransitionTime}>
                                            {condition.lastTransitionTime ? formatAge(condition.lastTransitionTime) : '-'}
                                        </span>
                                    </div>
                                ))}
                            </div>
                        </DetailSection>
                    )}

                    {/* Details */}
                    <DetailSection title="Details">
                        <DetailRow label="Name" value={name} />
                        {namespace && <DetailRow label="Namespace" value={namespace} />}
                        <DetailRow label="Kind" value={crdInfo.kind} />
                        <DetailRow label="API Version">
                            <span className="text-sm text-gray-300">
                                {crdInfo.group ? `${crdInfo.group}/${crdInfo.version}` : crdInfo.version}
                            </span>
                        </DetailRow>
                        <DetailRow label="Created">
                            <span title={resource.metadata?.creationTimestamp}>
                                {formatAge(resource.metadata?.creationTimestamp)} ago
                            </span>
                        </DetailRow>
                        <DetailRow label="UID">
                            <CopyableLabel value={uid?.substring(0, 8) + '...'} copyValue={uid} />
                        </DetailRow>
                    </DetailSection>

                    {/* Owner References */}
                    {ownerReferences.length > 0 && (
                        <DetailSection title="Owner References">
                            {ownerReferences.map((ref: any, idx: number) => (
                                <div key={idx} className="py-1.5 border-b border-border/50 last:border-0">
                                    <DetailRow label="Kind" value={ref.kind} />
                                    <DetailRow label="Name" value={ref.name} />
                                    <DetailRow label="API Version" value={ref.apiVersion} />
                                    {ref.controller && (
                                        <DetailRow label="Controller" value="true" />
                                    )}
                                </div>
                            ))}
                        </DetailSection>
                    )}

                    {/* Labels */}
                    <DetailSection title="Labels">
                        <LabelsDisplay labels={labels} />
                    </DetailSection>

                    {/* Annotations */}
                    <DetailSection title="Annotations">
                        <AnnotationsDisplay annotations={annotations} />
                    </DetailSection>
                </div>
            )}
        </div>
    );
}
