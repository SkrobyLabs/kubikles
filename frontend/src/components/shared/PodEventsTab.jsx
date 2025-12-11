import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { ExclamationTriangleIcon, InformationCircleIcon } from '@heroicons/react/24/outline';
import { ListEvents } from '../../../wailsjs/go/main/App';
import { useK8s } from '../../context/K8sContext';
import { useResourceWatcher } from '../../hooks/useResourceWatcher';
import { formatAge } from '../../utils/formatting';

// Event type indicator
const EventTypeIcon = ({ type }) => {
    if (type === 'Warning') {
        return <ExclamationTriangleIcon className="w-4 h-4 text-yellow-400" />;
    }
    return <InformationCircleIcon className="w-4 h-4 text-green-400" />;
};

export default function PodEventsTab({ pod, isStale }) {
    const { currentContext, lastRefresh } = useK8s();
    const [events, setEvents] = useState([]);
    const [loading, setLoading] = useState(false);

    const namespace = pod.metadata?.namespace;
    const podUid = pod.metadata?.uid;
    const podName = pod.metadata?.name;

    // Fetch events for the pod's namespace
    useEffect(() => {
        if (!currentContext || !namespace || isStale) return;

        const fetchEvents = async () => {
            setLoading(true);
            try {
                const list = await ListEvents(namespace);
                // Filter events related to this pod
                const podEvents = (list || []).filter(event =>
                    event.involvedObject?.uid === podUid ||
                    (event.involvedObject?.kind === 'Pod' && event.involvedObject?.name === podName)
                );
                setEvents(podEvents);
            } catch (err) {
                console.error('Failed to fetch events:', err);
            } finally {
                setLoading(false);
            }
        };

        fetchEvents();
    }, [currentContext, namespace, podUid, podName, isStale, lastRefresh]);

    // Handle real-time event updates
    const handleEvent = useCallback((event) => {
        const { type, resource } = event;

        // Only process events related to this pod
        const isRelevant = resource.involvedObject?.uid === podUid ||
            (resource.involvedObject?.kind === 'Pod' && resource.involvedObject?.name === podName);

        if (!isRelevant) return;

        setEvents(prev => {
            const uid = resource.metadata?.uid;
            if (!uid) return prev;

            switch (type) {
                case 'ADDED':
                    if (prev.find(e => e.metadata.uid === uid)) return prev;
                    return [...prev, resource];
                case 'MODIFIED':
                    return prev.map(e => e.metadata.uid === uid ? resource : e);
                case 'DELETED':
                    return prev.filter(e => e.metadata.uid !== uid);
                default:
                    return prev;
            }
        });
    }, [podUid, podName]);

    // Subscribe to event watcher for this namespace
    useResourceWatcher(
        'events',
        namespace ? [namespace] : [],
        handleEvent,
        currentContext && namespace && !isStale
    );

    // Sort events by last timestamp (most recent first)
    const sortedEvents = useMemo(() => {
        return [...events].sort((a, b) => {
            const timeA = new Date(a.lastTimestamp || a.metadata?.creationTimestamp || 0);
            const timeB = new Date(b.lastTimestamp || b.metadata?.creationTimestamp || 0);
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

    if (events.length === 0) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500">
                No events found for this pod
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
        </div>
    );
}
