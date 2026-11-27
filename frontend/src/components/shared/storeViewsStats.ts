// Pure helpers behind the Store tab of the performance panel.

export interface ShardStat {
    key: string;
    type: string;
    namespace: string;
    phase: string;
    mode: string;
    requested: string;
    degraded: boolean;
    degradedReason: string;
    entries: number;
    estimatedBytes: number;
    refs: number;
    lingering: boolean;
    subscribers: number;
    loaded: number;
    expected: number;
    error: string;
    forbidden: boolean;
}

export interface StoreStats {
    shards: ShardStat[];
    entries: number;
    estimatedBytes: number;
    maxBytes: number;
}

export interface ViewStat {
    id: string;
    type: string;
    namespaces: number;
    phase: string;
    version: number;
    total: number;
    filtered: number;
    windowRows: number;
    active: boolean;
    windows: number;
    rowsServed: number;
    notices: number;
    bytesSent: number;
    idleSecs: number;
}

export interface ViewRate {
    noticesPerMin: number;
    bytesPerMin: number;
}

/** Per-view rates from two readings of the cumulative counters. A view absent from the earlier reading has no rate yet. */
export function computeViewRates(
    previous: { timestamp?: number; views?: ViewStat[] } | null | undefined,
    current: { timestamp?: number; views?: ViewStat[] } | null | undefined,
): Map<string, ViewRate> {
    const out = new Map<string, ViewRate>();
    if (!previous || !current || !previous.views || !current.views) return out;
    const dtMs = (current.timestamp ?? 0) - (previous.timestamp ?? 0);
    if (dtMs <= 0) return out;
    const prev = new Map(previous.views.map(v => [v.id, v]));
    for (const v of current.views) {
        const p = prev.get(v.id);
        if (!p) continue;
        const perMin = 60000 / dtMs;
        out.set(v.id, {
            noticesPerMin: Math.max(0, v.notices - p.notices) * perMin,
            bytesPerMin: Math.max(0, v.bytesSent - p.bytesSent) * perMin,
        });
    }
    return out;
}

/** Shards that serve fewer or lighter objects than they were asked for are worth a flag. */
export function shardFlags(s: ShardStat): string[] {
    const flags: string[] = [];
    if (s.degraded) flags.push(s.degradedReason ? `degraded: ${s.degradedReason}` : 'degraded');
    if (s.lingering) flags.push('lingering');
    if (s.forbidden) flags.push('forbidden');
    if (s.error) flags.push(`error: ${s.error}`);
    return flags;
}
