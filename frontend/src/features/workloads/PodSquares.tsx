import React from 'react';
import { getPodStatusColor } from '~/utils/k8s-helpers';
import { useStoreValue, type ValueStore } from '~/utils/valueStore';
import { placeholderSquares } from './podDerivation';

export interface PodSquare {
    name: string;
    uid: string;
    status: string;
}

/** The squares of a windowed workload row, rebuilt from its display-only podRefs cell (JSON [name, uid, status] per pod). */
export function parsePodRefs(text: string | null | undefined): PodSquare[] {
    if (!text) return [];
    try {
        return (JSON.parse(text) as Array<[string, string, string]>).map(([name, uid, status]) => ({ name, uid, status }));
    } catch {
        return [];
    }
}

/**
 * The Pods cell of the deployment and stateful set lists: one small square per selected pod,
 * coloured by its effective status. While the pods have not loaded yet (the view reports the pod
 * source unavailable) and the workload shows none, placeholders stand in for its replicas, like
 * the legacy list does while its pod list is loading.
 */
export const PodSquares = React.memo(function PodSquares({ availability, refs, replicas }: {
    availability: ValueStore<boolean | null>;
    refs: string | null | undefined;
    replicas: number | null | undefined;
}) {
    const available = useStoreValue(availability, a => a);
    const pods = parsePodRefs(refs);
    if (available !== true && pods.length === 0) {
        const { count, more } = placeholderSquares(replicas);
        if (replicas === 0) return null;
        return (
            <div className="flex gap-1">
                {Array.from({ length: count }).map((_: unknown, i: number) => (
                    <div key={i} className="w-3 h-3 rounded-sm bg-gray-700 animate-pulse" title="Loading pods..." />
                ))}
                {more && <span className="text-xs text-gray-500">...</span>}
            </div>
        );
    }
    return (
        <div className="flex gap-1">
            {pods.map(pod => (
                <div
                    key={pod.uid}
                    className={`w-3 h-3 rounded-sm ${getPodStatusColor(pod.status).replace('text-', 'bg-')}`}
                    title={`${pod.name}: ${pod.status}`}
                />
            ))}
        </div>
    );
});
