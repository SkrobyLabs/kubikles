/** A pod the Evict action applies to: not terminating and not finished. */
export function isEvictablePod(pod: any): boolean {
    const isTerminating = !!pod.metadata?.deletionTimestamp;
    const isTerminal = pod.status?.phase === 'Succeeded' || pod.status?.phase === 'Failed';
    return !isTerminating && !isTerminal;
}

/** The pods an Evict modal lists; empty means the action must not open a modal. */
export function evictablePods(pods: any[]): any[] {
    return pods.filter(isEvictablePod);
}
