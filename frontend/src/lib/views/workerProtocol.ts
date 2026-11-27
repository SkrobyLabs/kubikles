import type { ObjectRef, PodTransition, RowRef, ViewNotice, ViewPatch, ViewSpec } from './protocol';
import type { ItemRef, ViewSnapshot } from './viewClient';
import type { RowItem } from './rowItem';

// Messages between the main thread (ViewHost, proxies) and the view worker (WorkerCore). Everything
// here must survive structured cloning: plain objects, arrays, strings, numbers and ArrayBuffers.

/** W: the worker fetches itself. M: the main thread fetches and transfers the bytes. */
export type WorkerMode = 'W' | 'M';

export const HEARTBEAT_MS = 2000;
export const ACK_TIMEOUT_MS = 2000;
export const MISSED_HEARTBEATS = 3;
/** Refs lists longer than this travel as a transferred index plus string table. */
export const PACKED_REFS_THRESHOLD = 10_000;

export interface WorkerInit {
    t: 'init';
    /** protocol//host: relative URLs do not resolve inside blob workers. */
    origin: string;
    base: string;
    token: string;
    mode: WorkerMode;
    /** Bodies arrive incrementally (macOS, server mode); otherwise bodies are read whole. */
    streaming: boolean;
    heartbeatMs: number;
}

export type RequestOp = 'refs' | 'selectAll' | 'facet' | 'getObject' | 'getObjects' | 'open';

export type ToWorker =
    | WorkerInit
    | { t: 'view.open'; rid: number; id: string; spec: ViewSpec }
    | { t: 'view.update'; id: string; patch: ViewPatch }
    | { t: 'view.close'; id: string }
    | { t: 'view.setRange'; id: string; start: number; end: number }
    | { t: 'view.setSelection'; id: string; uids: string[] }
    | { t: 'view.setActive'; id: string; active: boolean }
    | { t: 'view.refresh'; id: string }
    | { t: 'view.reopen'; id: string }
    | { t: 'req'; rid: number; id: string; op: Exclude<RequestOp, 'open'>; args: unknown[] }
    | { t: 'notice'; notices: ViewNotice[] }
    | { t: 'reconnect' }
    | { t: 'fetch.call'; rid: number; path: string }
    | { t: 'rpc.result'; rid: number; ok: boolean; value?: unknown; error?: WireError }
    | { t: 'dispose' };

export interface WireError {
    message: string;
    gone?: boolean;
    status?: number;
}

export interface WorkerStats {
    views: number;
    rowsResident: number;
    /** Cumulative milliseconds spent in JSON and NDJSON parsing inside the worker. */
    decodeMs: number;
    fetches: number;
    fetchFailures: number;
}

/** One row on the wire to the main thread; RowItem is already plain data. */
export type WireRow = RowItem;

export interface PackedRefs {
    packed: true;
    count: number;
    /** Uint32 triples (uid, ns, name) indexing into the table. */
    index: ArrayBuffer;
    /** Unique strings joined with NUL. */
    table: string;
}

export type RefsResult = RowRef[] | PackedRefs;

export type RpcOp =
    | 'open' | 'update' | 'close' | 'setActive' | 'window' | 'refs' | 'facet' | 'setSelection'
    | 'getObject' | 'getObjects' | 'bytes';

export interface FetchedCall {
    status: number;
    ok: boolean;
    /** Parsed NDJSON lines when the body was application/x-ndjson. */
    items?: unknown[];
    /** Parsed JSON body otherwise (null when the body was not JSON). */
    doc?: { data?: unknown; error?: string } | null;
}

export type FromWorker =
    | { t: 'ack' }
    | { t: 'heartbeat'; stats: WorkerStats }
    | { t: 'view.state'; id: string; snapshot: ViewSnapshot }
    | {
        t: 'view.rows';
        id: string;
        /** Index of order[0]. */
        start: number;
        /** uid per index from start; '' where the row is not loaded. */
        order: string[];
        rows: WireRow[];
    }
    | { t: 'view.stale'; id: string; uids: string[] }
    | { t: 'view.transitions'; id: string; events: PodTransition[] }
    | { t: 'view.error'; id: string; error: string }
    | { t: 'res'; rid: number; ok: boolean; value?: unknown; error?: WireError }
    | { t: 'rpc'; rid: number; op: RpcOp; args: unknown[] }
    /** Worker fetches keep failing: the host should step down to level M. */
    | { t: 'fetch.failing'; error: string };

export type { ItemRef, ObjectRef };

export function toWireError(e: unknown, gone: boolean): WireError {
    const status = (e as { status?: number } | null)?.status;
    const message = typeof e === 'string' ? e : (e as Error | null)?.message ?? String(e);
    return { message, ...(gone ? { gone: true } : {}), ...(typeof status === 'number' ? { status } : {}) };
}

export function packRefs(refs: RowRef[]): PackedRefs {
    const seen = new Map<string, number>();
    const strings: string[] = [];
    const idx = new Uint32Array(refs.length * 3);
    const intern = (s: string): number => {
        let i = seen.get(s);
        if (i === undefined) {
            i = strings.length;
            seen.set(s, i);
            strings.push(s);
        }
        return i;
    };
    for (let i = 0; i < refs.length; i++) {
        idx[i * 3] = intern(refs[i].uid);
        idx[i * 3 + 1] = intern(refs[i].ns);
        idx[i * 3 + 2] = intern(refs[i].name);
    }
    return { packed: true, count: refs.length, index: idx.buffer as ArrayBuffer, table: strings.join('\u0000') };
}

export function unpackRefs(p: PackedRefs): RowRef[] {
    const table = p.table.length === 0 && p.count === 0 ? [] : p.table.split('\u0000');
    const idx = new Uint32Array(p.index);
    const out: RowRef[] = new Array(p.count);
    for (let i = 0; i < p.count; i++) {
        out[i] = { uid: table[idx[i * 3]], ns: table[idx[i * 3 + 1]], name: table[idx[i * 3 + 2]] };
    }
    return out;
}

export function isPackedRefs(x: unknown): x is PackedRefs {
    return !!x && typeof x === 'object' && (x as PackedRefs).packed === true;
}
