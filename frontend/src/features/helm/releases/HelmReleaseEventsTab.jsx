import React, { useState, useEffect, useMemo, useCallback } from 'react';
import { ExclamationTriangleIcon, InformationCircleIcon } from '@heroicons/react/24/outline';
import { ListEvents, GetHelmReleaseResources } from '../../../../wailsjs/go/main/App';
import { useK8s } from '../../../context/K8sContext';
import { useResourceWatcher } from '../../../hooks/useResourceWatcher';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

// Event type indicator
const EventTypeIcon = ({ type }) => {
    if (type === 'Warning') {
        return <ExclamationTriangleIcon className="w-4 h-4 text-yellow-400" />;
    }
    return <InformationCircleIcon className="w-4 h-4 text-green-400" />;
};

export default function HelmReleaseEventsTab({ release, isStale }) {
    const { currentContext, lastRefresh } = useK8s();
    const [events, setEvents] = useState([]);
    const [resources, setResources] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    const namespace = release?.namespace;
    const releaseName = release?.name;

    // Fetch release resources and their events
    useEffect(() => {
        if (!currentContext || !namespace || !releaseName || isStale) return;

        const fetchData = async () => {
            setLoading(true);
            setError(null);
            try {
                Logger.info("Fetching Helm release resources", { namespace, name: releaseName });

                // First get the resources managed by this release
                const releaseResources = await GetHelmReleaseResources(namespace, releaseName);
                setResources(releaseResources || []);

                // Get unique namespaces from resources (some might be in different namespaces)
                const resourceNamespaces = new Set([namespace]);
                (releaseResources || []).forEach(r => {
                    if (r.namespace) resourceNamespaces.add(r.namespace);
                });

                // Fetch events from all relevant namespaces
                let allEvents = [];
                for (const ns of resourceNamespaces) {
                    const nsEvents = await ListEvents(ns);
                    allEvents = allEvents.concat(nsEvents || []);
                }

                // Filter events related to release resources
                const releaseEvents = allEvents.filter(event => {
                    const involvedObj = event.involvedObject;
                    if (!involvedObj) return false;

                    return (releaseResources || []).some(res =>
                        res.kind === involvedObj.kind &&
                        res.name === involvedObj.name &&
                        (res.namespace === involvedObj.namespace || !involvedObj.namespace)
                    );
                });

                setEvents(releaseEvents);
            } catch (err) {
                Logger.error("Failed to fetch release events", err);
                setError(err.message || String(err));
            } finally {
                setLoading(false);
            }
        };

        fetchData();
    }, [currentContext, namespace, releaseName, isStale, lastRefresh]);

    // Handle real-time event updates
    const handleEvent = useCallback((event) => {
        const { type, resource: eventResource } = event;

        // Only process events related to release resources
        const involvedObj = eventResource.involvedObject;
        if (!involvedObj) return;

        const isRelevant = resources.some(res =>
            res.kind === involvedObj.kind &&
            res.name === involvedObj.name &&
            (res.namespace === involvedObj.namespace || !involvedObj.namespace)
        );

        if (!isRelevant) return;

        setEvents(prev => {
            const uid = eventResource.metadata?.uid;
            if (!uid) return prev;

            switch (type) {
                case 'ADDED':
                    if (prev.find(e => e.metadata?.uid === uid)) return prev;
                    return [...prev, eventResource];
                case 'MODIFIED':
                    return prev.map(e => e.metadata?.uid === uid ? eventResource : e);
                case 'DELETED':
                    return prev.filter(e => e.metadata?.uid !== uid);
                default:
                    return prev;
            }
        });
    }, [resources]);

    // Subscribe to event watcher for the release namespace
    useResourceWatcher(
        'events',
        namespace ? [namespace] : [],
        handleEvent,
        currentContext && namespace && !isStale && resources.length > 0
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
                <div className="flex items-center gap-3">
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                    <span>Loading events...</span>
                </div>
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex items-center justify-center h-full text-red-400">
                {error}
            </div>
        );
    }

    if (events.length === 0) {
        return (
            <div className="flex flex-col items-center justify-center h-full text-gray-500">
                <span>No events found for this release</span>
                {resources.length > 0 && (
                    <span className="text-xs mt-1">
                        Monitoring {resources.length} resource{resources.length !== 1 ? 's' : ''}
                    </span>
                )}
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
                            <th className="px-4 py-2">Resource</th>
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
                                <td className="px-4 py-2 font-mono text-xs text-gray-400">
                                    <span className="text-gray-500">{event.involvedObject?.kind}/</span>
                                    {event.involvedObject?.name}
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

            {/* Resource count indicator */}
            <div className="px-4 py-1 text-xs text-gray-500 border-t border-border bg-surface/50">
                {events.length} event{events.length !== 1 ? 's' : ''} from {resources.length} resource{resources.length !== 1 ? 's' : ''}
            </div>
        </div>
    );
}
