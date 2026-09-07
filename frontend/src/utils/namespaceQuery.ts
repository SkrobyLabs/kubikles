// Query scope is independent of the namespace filter shown to the user.
export const NAMESPACE_QUERY_CONCURRENCY = 4;
const MiB = 1024 * 1024;
const OBSERVATION_TTL = 5 * 60_000;

export interface NamespaceQueryObservation {
    count: number;
    bytes: number;
    durationMs: number;
    observedAt: number;
}

export function namespaceSelection(value: string | string[] | null | undefined): string[] {
    if (value == null) return [];
    const values = Array.isArray(value) ? value : [value];
    if (values.includes('*') || values.includes('')) return ['*'];
    return [...new Set(values)].sort();
}

export function chooseNamespaceScope(
    selected: string[], available: string[],
    observation?: NamespaceQueryObservation,
    options: { broadDenied?: boolean; reuseBroad?: boolean; scopedDurationMs?: number; now?: number } = {},
): string[] {
    if (!selected.length) return [];
    if (selected.includes('*')) return [''];
    if (options.broadDenied) return selected;
    const known = new Set(available.filter(ns => ns && ns !== '*'));
    const coverage = known.size ? selected.filter(ns => known.has(ns)).length / known.size : 0;
    const fresh = observation && (options.now ?? Date.now()) - observation.observedAt < OBSERVATION_TTL;
    if (fresh) {
        // Object count alone misses large secrets/configmaps. Include payload size and latency.
        const small = observation.count <= 500 && observation.bytes <= 5 * MiB && observation.durationMs <= 2000;
        if (small && (selected.length >= 2 || options.reuseBroad)) return [''];
        const scopedEstimate = options.scopedDurationMs === undefined ? 2000
            : Math.ceil(selected.length / NAMESPACE_QUERY_CONCURRENCY) * options.scopedDurationMs;
        if (selected.length >= 4 && coverage >= 0.8 && observation.count <= 5000 &&
            observation.bytes <= 20 * MiB && observation.durationMs <= scopedEstimate) return [''];
        return selected;
    }
    // No cluster-wide counts yet: don't infer density from a few possibly empty namespaces.
    return selected.length >= 4 && coverage >= 0.5 ? [''] : selected;
}

export function observeNamespaceList(items: unknown[], durationMs: number): NamespaceQueryObservation {
    // This is a query-planning estimate, not a transport measurement. Bound work
    // independently of cluster size and sample across the complete list.
    const sampleSize = Math.min(items.length, 32);
    const encoder = new TextEncoder();
    let largest = 0;
    let total = 2; // JSON array brackets
    for (let i = 0; i < sampleSize; i++) {
        const index = sampleSize === items.length ? i : Math.floor(i * items.length / sampleSize);
        const bytes = encoder.encode(JSON.stringify(items[index]) ?? 'null').byteLength;
        largest = Math.max(largest, bytes);
        total += bytes + (i ? 1 : 0);
    }
    // Prefer a conservative estimate when sampling heterogeneous objects.
    const bytes = items.length <= sampleSize ? total : 2 + items.length * (largest + 1);
    return { count: items.length, bytes, durationMs, observedAt: Date.now() };
}

export function isNamespaceForbidden(error: unknown): boolean {
    return /\bforbidden\b|\b403\b/i.test(String(error));
}

/** An atomic list: failures never masquerade as successful empty namespaces. */
export async function listNamespaceScope<T>(
    namespaces: string[], list: (namespace: string) => Promise<T[]>, isCurrent: () => boolean = () => true,
): Promise<T[]> {
    let next = 0;
    let failed = false;
    const results: T[][] = new Array(namespaces.length);
    const workers = Array.from({ length: Math.min(NAMESPACE_QUERY_CONCURRENCY, namespaces.length) }, async () => {
        while (!failed && isCurrent() && next < namespaces.length) {
            const index = next++;
            try {
                results[index] = await list(namespaces[index]);
            } catch (error) {
                failed = true;
                throw error;
            }
        }
    });
    // Drain running workers before the caller can retry or start another polling cycle.
    const settled = await Promise.allSettled(workers);
    const rejection = settled.find((result): result is PromiseRejectedResult => result.status === 'rejected');
    if (rejection) throw rejection.reason;
    return results.flat().filter(Boolean);
}
