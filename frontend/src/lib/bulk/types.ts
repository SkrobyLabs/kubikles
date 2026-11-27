export interface BulkInfo {
    enabled: boolean;
    base: string;
    token: string;
    streaming: boolean;
    server: boolean;
    version: number;
}

export interface BulkBenchMs {
    bulkJson: number | null;
    bulkNdjson: number | null;
    ipc: number | null;
}

/** What this WebView can do, as measured by the probe. */
export interface BulkCaps {
    /** Backend allows the transport (setting, env). */
    enabled: boolean;
    /** fetch() to /_kubikles works from the main thread. */
    fetch: boolean;
    /** Lines arrive incrementally (first line well before the last). */
    streaming: boolean;
    /** Bodies only arrive complete (Windows desktop writer). */
    buffered: boolean;
    /** A Blob worker can be constructed and replies. */
    workerBlob: boolean;
    /** A Blob worker can fetch the bulk routes. */
    workerFetch: boolean;
    /** A classic worker from a same-origin script URL works (informational). */
    workerScriptUrl: boolean;
    /** Too many consecutive failures; routing is paused until a re-probe. */
    degraded: boolean;
    notes: string[];
    probedAt: number;
    userAgent: string;
    benchMs: BulkBenchMs;
    /** True in server mode (browser client). */
    server?: boolean;
    /** Main-thread long task time per bench run, ms. */
    longTaskMs?: BulkBenchMs;
    /** Transfer time for the bytes probes, keyed by MB. */
    bytesMs?: Record<string, number>;
    /** True when restored from the localStorage summary instead of measured. */
    cached?: boolean;
}

export interface BulkStats {
    calls: number;
    fallbacks: number;
    failures: number;
    consecutiveFailures: number;
    lastError: string;
}
