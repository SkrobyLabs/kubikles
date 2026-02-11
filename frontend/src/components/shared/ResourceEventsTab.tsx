import React, { useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { ExclamationTriangleIcon, InformationCircleIcon } from '@heroicons/react/24/outline';
import { ListEvents, ListPods } from 'wailsjs/go/main/App';
import { useK8s } from '~/context';
import { useResourceWatcher } from '~/hooks/useResourceWatcher';
import { formatAge } from '~/utils/formatting';

// Event type indicator
const EventTypeIcon = ({ type }: { type: string }) => {
    if (type === 'Warning') {
        return <ExclamationTriangleIcon className="w-4 h-4 text-yellow-400" />;
    }
    return <InformationCircleIcon className="w-4 h-4 text-green-400" />;
};

interface ResourceEventsTabProps {
    kind: string;
    namespace: string;
    name: string;
    uid: string;
    isStale: boolean;
    /** When provided, also includes events from pods matching these labels (for controllers) */
    matchLabels?: Record<string, string>;
}

/** Check if a pod's labels match all the required matchLabels */
function labelsMatch(podLabels: Record<string, string> | undefined, matchLabels: Record<string, string>): boolean {
    if (!podLabels) return false;
    return Object.entries(matchLabels).every(([k, v]) => podLabels[k] === v);
}

export default function ResourceEventsTab({ kind, namespace, name, uid, isStale, matchLabels }: ResourceEventsTabProps) {
    const { currentContext, lastRefresh } = useK8s();
    const [events, setEvents] = useState<any[]>([]);
    const [loading, setLoading] = useState(false);
    // Set of related UIDs (child pods) for matching events
    const relatedUidsRef = useRef<Set<string>>(new Set());

    // Fetch events for the resource's namespace
    useEffect(() => {
        if (!currentContext || !namespace || isStale) return;

        const fetchEvents = async () => {
            setLoading(true);
            try {
                // If matchLabels provided, fetch child pods to build related UID set
                const relatedUids = new Set<string>();
                if (matchLabels && Object.keys(matchLabels).length > 0) {
                    try {
                        const pods = await ListPods('', namespace);
                        for (const pod of (pods || [])) {
                            if (labelsMatch(pod.metadata?.labels, matchLabels)) {
                                if (pod.metadata?.uid) relatedUids.add(pod.metadata.uid);
                            }
                        }
                    } catch {
                        // Non-fatal: proceed without child pod events
                    }
                }
                relatedUidsRef.current = relatedUids;

                const list = await ListEvents('', namespace);
                // Filter events related to this resource by UID or by kind+name,
                // plus events for child pods when matchLabels is provided
                const resourceEvents = (list || []).filter((event: any) => {
                    const obj = event.involvedObject;
                    if (!obj) return false;
                    if (obj.uid === uid) return true;
                    if (obj.kind === kind && obj.name === name) return true;
                    if (relatedUids.size > 0 && obj.uid && relatedUids.has(obj.uid)) return true;
                    return false;
                });
                setEvents(resourceEvents);
            } catch (err: any) {
                console.error('Failed to fetch events:', err);
            } finally {
                setLoading(false);
            }
        };

        fetchEvents();
    }, [currentContext, namespace, uid, name, kind, isStale, lastRefresh, matchLabels]);

    // Handle real-time event updates
    const handleEvent = useCallback((event: any) => {
        const { type, resource } = event;
        const obj = resource.involvedObject;

        // Only process events related to this resource or its child pods
        const isRelevant = obj?.uid === uid ||
            (obj?.kind === kind && obj?.name === name) ||
            (relatedUidsRef.current.size > 0 && obj?.uid && relatedUidsRef.current.has(obj.uid));

        if (!isRelevant) return;

        setEvents(prev => {
            const eventUid = resource.metadata?.uid;
            if (!eventUid) return prev;

            switch (type) {
                case 'ADDED':
                    if (prev.find((e: any) => e.metadata.uid === eventUid)) return prev;
                    return [...prev, resource];
                case 'MODIFIED':
                    return prev.map((e: any) => e.metadata.uid === eventUid ? resource : e);
                case 'DELETED':
                    return prev.filter((e: any) => e.metadata.uid !== eventUid);
                default:
                    return prev;
            }
        });
    }, [uid, name, kind]);

    // Subscribe to event watcher for this namespace
    useResourceWatcher(
        'events',
        namespace ? [namespace] : [],
        handleEvent,
        !!(currentContext && namespace && !isStale)
    );

    // Sort events by last timestamp (most recent first)
    const sortedEvents = useMemo(() => {
        return [...events].sort((a, b) => {
            const timeA = new Date(a.lastTimestamp || a.metadata?.creationTimestamp || 0).getTime();
            const timeB = new Date(b.lastTimestamp || b.metadata?.creationTimestamp || 0).getTime();
            return timeB - timeA;
        });
    }, [events]);

    if (loading && events.length === 0) {
        return (
            <div className="flex items-center justify-center h-full text-gray-400">
                Loading events...
            </div>
        );
    }

    const showObjectColumn = !!matchLabels;

    if (events.length === 0) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500">
                No events found for this {kind.toLowerCase()}
            </div>
        );
    }

    return (
        <div className="flex flex-col h-full overflow-hidden">
            {/* Events List */}
            <div className="flex-1 overflow-auto">
                <table className="w-full text-sm">
                    <thead className="sticky top-0 bg-surface border-b border-border">
                        <tr className="text-left text-xs text-gray-500 uppercase tracking-wider">
                            <th className="px-4 py-2 w-10"></th>
                            {showObjectColumn && <th className="px-4 py-2 w-40">Object</th>}
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
                                {showObjectColumn && (
                                    <td className="px-4 py-2 text-xs text-gray-500 truncate" title={`${event.involvedObject?.kind}/${event.involvedObject?.name}`}>
                                        <span className="text-gray-400">{event.involvedObject?.kind}/</span>
                                        {event.involvedObject?.name}
                                    </td>
                                )}
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
        </div>
    );
}
