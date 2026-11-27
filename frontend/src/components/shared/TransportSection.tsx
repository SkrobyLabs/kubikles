import { useEffect, useState, useSyncExternalStore } from 'react';
import { getBulkStats, getBulkVersion, peekBulkCaps, subscribeBulk } from '~/lib/bulk/transport';
import { hostLevel, runBulkProbe } from '~/lib/bulk/probe';
import { getViewFactory } from '~/lib/views/viewClientFactory';

const yesNo = (v: boolean) => (v ? 'yes' : 'no');

const Row = ({ label, value }: { label: string; value: string | number }) => (
    <div className="flex justify-between gap-4 py-1 border-b border-border/50 last:border-0 text-xs">
        <span className="text-gray-400">{label}</span>
        <span className="text-text font-mono text-right break-all">{value}</span>
    </div>
);

const LEVEL_TEXT = {
    W: 'W (worker fetches and decodes)',
    M: 'M (worker decodes, main thread fetches)',
    I: 'I (inline on the main thread)',
} as const;

/** Host level, open views, notice rate, worker decode time, resident rows and watchdog events. */
function ViewWorkerSection() {
    const factory = getViewFactory();
    useSyncExternalStore(
        (cb) => factory.subscribe(cb),
        () => factory.getVersion(),
        () => factory.getVersion(),
    );
    // Rates and worker counters change without a version bump, so sample once a second.
    const [, setTick] = useState(0);
    useEffect(() => {
        const t = setInterval(() => setTick((n) => n + 1), 1000);
        return () => clearInterval(t);
    }, []);
    const st = factory.status();
    return (
        <div className="bg-surface-light rounded-lg border border-border p-3 mb-2">
            <Row label="View engine level" value={st.level ? LEVEL_TEXT[st.level] : 'not used yet'} />
            <Row label="Setting / highest level allowed" value={`${st.setting} / ${st.ceiling}`} />
            <Row label="Views open / total (this window)" value={`${st.viewsOpen} / ${st.views}`} />
            <Row label="Notices per second" value={st.noticesPerSec} />
            <Row label="Worker decode time" value={st.worker ? `${st.worker.decodeMs} ms` : 'n/a'} />
            <Row label="Rows resident in worker" value={st.worker ? st.worker.rowsResident : 'n/a'} />
            <Row label="Worker fetches / failures (all bulk calls, lists without a view too)" value={st.worker ? `${st.worker.fetches} / ${st.worker.fetchFailures}` : 'n/a'} />
            <Row label="Watchdog events" value={st.watchdog.length} />
            {st.watchdog.map((w, i) => (
                <Row key={i} label={new Date(w.at).toLocaleTimeString()} value={`${w.mode} -> ${w.to}: ${w.reason}`} />
            ))}
        </div>
    );
}

/** Read-only view of the bulk transport capabilities and counters. */
export default function TransportSection() {
    useSyncExternalStore(subscribeBulk, getBulkVersion, getBulkVersion);
    const [running, setRunning] = useState(false);
    const [copied, setCopied] = useState(false);
    const caps = peekBulkCaps();
    const stats = getBulkStats();
    const active = caps.enabled && caps.fetch && !caps.degraded ? 'bulk fetch (bindings as fallback)' : 'bindings';

    const bench = caps.benchMs;
    const ms = (v: number | null) => (v === null ? 'n/a' : `${v} ms`);
    const json = JSON.stringify({ caps, stats, hostLevel: hostLevel(caps) }, null, 2);

    const run = async () => {
        setRunning(true);
        try {
            await runBulkProbe({ force: true });
        } finally {
            setRunning(false);
        }
    };
    const copy = async () => {
        try {
            await navigator.clipboard.writeText(json);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        } catch {
            // clipboard unavailable
        }
    };

    return (
        <div className="mb-5">
            <h3 className="text-xs font-semibold text-gray-400 mb-2 uppercase tracking-wider">Transport</h3>
            <div className="bg-surface-light rounded-lg border border-border p-3 mb-2">
                <Row label="Active transport" value={active} />
                <Row label="Enabled by backend" value={yesNo(caps.enabled)} />
                <Row label="Fetch works" value={yesNo(caps.fetch)} />
                <Row label="Streaming" value={caps.streaming ? 'yes' : caps.buffered ? 'no (buffered)' : 'unknown'} />
                <Row label="Blob worker / worker fetch / script worker" value={`${yesNo(caps.workerBlob)} / ${yesNo(caps.workerFetch)} / ${yesNo(caps.workerScriptUrl)}`} />
                <Row label="Host level" value={hostLevel(caps)} />
                <Row label="Degraded" value={yesNo(caps.degraded)} />
                <Row label="Bench (JSON / NDJSON / IPC)" value={`${ms(bench.bulkJson)} / ${ms(bench.bulkNdjson)} / ${ms(bench.ipc)}`} />
                <Row label="Calls / fallbacks / failures" value={`${stats.calls} / ${stats.fallbacks} / ${stats.failures}`} />
                {stats.lastError && <Row label="Last error" value={stats.lastError} />}
                {caps.probedAt > 0 && <Row label="Probed" value={`${new Date(caps.probedAt).toLocaleString()}${caps.cached ? ' (cached)' : ''}`} />}
                {caps.notes.map((n, i) => (
                    <Row key={i} label="Note" value={n} />
                ))}
            </div>
            <div className="flex gap-2 mb-2">
                <button
                    type="button"
                    onClick={run}
                    disabled={running}
                    className="px-2 py-1 text-xs rounded bg-surface-light border border-border text-text hover:bg-surface disabled:opacity-50"
                >
                    {running ? 'Probing...' : 'Run probe'}
                </button>
                <button
                    type="button"
                    onClick={copy}
                    className="px-2 py-1 text-xs rounded bg-surface-light border border-border text-text hover:bg-surface"
                >
                    {copied ? 'Copied' : 'Copy JSON'}
                </button>
            </div>
            <h3 className="text-xs font-semibold text-gray-400 mb-2 mt-4 uppercase tracking-wider">View worker</h3>
            <ViewWorkerSection />
            <pre className="bg-surface-light rounded-lg border border-border p-3 text-xs font-mono text-gray-300 max-h-64 overflow-auto">{json}</pre>
        </div>
    );
}
