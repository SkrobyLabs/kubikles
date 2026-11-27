import React, { useMemo } from 'react';
import { computeViewRates, shardFlags, type StoreStats, type ViewStat } from './storeViewsStats';

const formatBytes = (bytes: number) => {
    if (!bytes) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.min(sizes.length - 1, Math.floor(Math.log(bytes) / Math.log(k)));
    return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
};

const Cell = ({ children, mono = true, className = '' }: { children: React.ReactNode; mono?: boolean; className?: string }) => (
    <td className={`px-2 py-1 text-xs ${mono ? 'font-mono' : ''} text-gray-300 whitespace-nowrap ${className}`}>{children}</td>
);

const Head = ({ children }: { children: React.ReactNode }) => (
    <th className="px-2 py-1 text-left text-xs font-medium text-gray-400 whitespace-nowrap">{children}</th>
);

interface Props {
    metrics: { timestamp?: number; store?: StoreStats; views?: ViewStat[] } | null | undefined;
    previous?: { timestamp?: number; views?: ViewStat[] } | null;
}

/** Shared store shards and open windowed views, from the backend performance metrics. */
export default function StoreViewsSection({ metrics, previous = null }: Props) {
    const store = metrics?.store;
    // The backend sends null for an empty list on builds that predate the fix there.
    const shards = store?.shards ?? [];
    const views = metrics?.views ?? [];
    const rates = useMemo(() => computeViewRates(previous, metrics), [previous, metrics]);

    return (
        <>
            <div className="mb-5">
                <h3 className="text-xs font-semibold text-gray-400 mb-2 uppercase tracking-wider">Store shards</h3>
                {!store ? (
                    <div className="text-gray-500 text-sm">The store is not running.</div>
                ) : (
                    <div className="bg-surface-light rounded-lg border border-border overflow-auto">
                        <div className="px-3 py-2 text-xs text-gray-400 border-b border-border/50" data-testid="store-summary">
                            {shards.length} shards, {store.entries} entries, {formatBytes(store.estimatedBytes)} estimated
                            {store.maxBytes > 0 ? ` of ${formatBytes(store.maxBytes)} budget` : ' (no budget)'}
                        </div>
                        <table className="w-full">
                            <thead>
                                <tr><Head>Type</Head><Head>Namespace</Head><Head>Phase</Head><Head>Mode</Head><Head>Entries</Head><Head>Estimated</Head><Head>Leases</Head><Head>Notes</Head></tr>
                            </thead>
                            <tbody>
                                {shards.map(s => (
                                    <tr key={s.key} className="border-t border-border/50">
                                        <Cell>{s.type}</Cell>
                                        <Cell>{s.namespace || 'all'}</Cell>
                                        <Cell>{s.phase === 'loading' && s.expected > 0 ? `loading ${s.loaded}/${s.expected}` : s.phase}</Cell>
                                        <Cell>{s.mode === s.requested ? s.mode : `${s.mode} (asked ${s.requested})`}</Cell>
                                        <Cell>{s.entries}</Cell>
                                        <Cell>{formatBytes(s.estimatedBytes)}</Cell>
                                        <Cell>{s.refs}</Cell>
                                        <Cell mono={false} className="whitespace-normal">{shardFlags(s).join('; ') || '-'}</Cell>
                                    </tr>
                                ))}
                                {shards.length === 0 && (
                                    <tr><Cell mono={false}>No shards are open.</Cell></tr>
                                )}
                            </tbody>
                        </table>
                    </div>
                )}
            </div>

            <div className="mb-5">
                <h3 className="text-xs font-semibold text-gray-400 mb-2 uppercase tracking-wider">Windowed views</h3>
                <div className="bg-surface-light rounded-lg border border-border overflow-auto">
                    <table className="w-full">
                        <thead>
                            <tr><Head>Type</Head><Head>Phase</Head><Head>Rows</Head><Head>Window</Head><Head>Version</Head><Head>Notices/min</Head><Head>Sent/min</Head><Head>State</Head></tr>
                        </thead>
                        <tbody>
                            {views.map(v => {
                                const r = rates.get(v.id);
                                return (
                                    <tr key={v.id} className="border-t border-border/50">
                                        <Cell>{v.type}</Cell>
                                        <Cell>{v.phase}</Cell>
                                        <Cell>{v.filtered === v.total ? v.total : `${v.filtered} of ${v.total}`}</Cell>
                                        <Cell>{v.windowRows}</Cell>
                                        <Cell>{v.version}</Cell>
                                        <Cell>{r ? r.noticesPerMin.toFixed(1) : '-'}</Cell>
                                        <Cell>{r ? formatBytes(r.bytesPerMin) : '-'}</Cell>
                                        <Cell>{v.active ? 'active' : 'hidden'}</Cell>
                                    </tr>
                                );
                            })}
                            {views.length === 0 && (
                                <tr><Cell mono={false}>No views are open. A windowed list holds a view only while its list is on screen and Windowed Lists is enabled in settings; views are counted for the whole process, not per window.</Cell></tr>
                            )}
                        </tbody>
                    </table>
                </div>
            </div>
        </>
    );
}
