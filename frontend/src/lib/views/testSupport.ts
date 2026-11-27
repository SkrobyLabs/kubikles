import { vi } from 'vitest';
import { ViewGoneError, ViewStaleError, type RowRef, type ViewNotice, type ViewOpenResult, type ViewPatch, type ViewSpec, type ViewState, type ViewWindow } from './protocol';
import type { NoticeFeed, ViewTransport } from './transport';

export interface FakeItem {
    uid: string;
    ns: string;
    name: string;
    seq: number;
    cells?: unknown[];
}

export const COLUMNS = ['name', 'age'];

/**
 * A scripted backend. It keeps the rows, a version counter and the last served window, and
 * answers window calls with the same `have` rule the Go side uses: rows newer than `have`,
 * plus any row that was not in the last served window.
 */
export class FakeBackend implements ViewTransport {
    items: FakeItem[] = [];
    version = 1;
    exists = false;
    openCalls: ViewSpec[] = [];
    updateCalls: Array<{ id: string; patch: ViewPatch }> = [];
    windowCalls: Array<{ offset: number; limit: number; have: number }> = [];
    closeCalls: string[] = [];
    activeCalls: boolean[] = [];
    selectionCalls: string[][] = [];
    refsCalls: Array<{ offset: number; limit: number; version?: number }> = [];
    /** Number of upcoming window calls that never answer on their own (they only end when aborted). */
    hangWindows = 0;
    windowSignals: Array<AbortSignal | undefined> = [];
    /** Makes the next refs call fail as if the list had moved on. */
    staleRefs = false;
    activeError: unknown = null;
    failOpen = 0;
    windowError: unknown = null;
    selectionStale: string[] = [];
    columns = [...COLUMNS];
    /** Row schema returned with the open result (dynamic types). */
    schema: ViewOpenResult['schema'] = undefined;
    phase: ViewState['phase'] = 'complete';
    private lastServed = new Set<string>();
    private gate: Promise<void> | null = null;
    private release: (() => void) | null = null;

    /** Holds window responses until open() is called, to script ordering. */
    hold(): void {
        this.gate = new Promise(r => { this.release = r; });
    }

    releaseHold(): void {
        this.release?.();
        this.gate = null;
    }

    add(n: number, prefix = 'r'): void {
        const start = this.items.length;
        for (let i = 0; i < n; i++) {
            this.items.push({ uid: `${prefix}${start + i}`, ns: 'ns', name: `${prefix}${start + i}`, seq: ++this.version });
        }
    }

    touch(uid: string): void {
        const it = this.items.find(i => i.uid === uid)!;
        it.seq = ++this.version;
    }

    state(): ViewState {
        return { version: this.version, total: this.items.length, filtered: this.items.length, phase: this.phase, loaded: this.items.length, expected: this.items.length };
    }

    async open(spec: ViewSpec): Promise<ViewOpenResult> {
        this.openCalls.push(spec);
        if (this.failOpen > 0) {
            this.failOpen--;
            throw new Error('open failed');
        }
        this.exists = true;
        this.lastServed.clear();
        return { viewId: spec.viewId, columns: [...this.columns], schemaVersion: 'sv', state: this.state(), filterable: {}, ...(this.schema ? { schema: this.schema } : {}) };
    }

    async update(id: string, patch: ViewPatch): Promise<ViewState> {
        this.updateCalls.push({ id, patch });
        if (!this.exists) throw new ViewGoneError();
        if (patch.columns) this.columns = [...new Set(patch.columns)];
        this.version++;
        this.lastServed.clear();
        return this.state();
    }

    async close(id: string): Promise<void> {
        this.closeCalls.push(id);
    }

    async setActive(_id: string, active: boolean): Promise<void> {
        this.activeCalls.push(active);
        if (this.activeError) throw this.activeError;
    }

    async window(_id: string, offset: number, limit: number, have: number, signal?: AbortSignal): Promise<ViewWindow> {
        this.windowCalls.push({ offset, limit, have });
        this.windowSignals.push(signal);
        if (this.hangWindows > 0) {
            this.hangWindows--;
            await new Promise<never>((_, reject) => {
                signal?.addEventListener('abort', () => reject(new DOMException('aborted', 'AbortError')));
            });
        }
        if (this.gate) await this.gate;
        if (this.windowError) throw this.windowError;
        if (!this.exists) throw new ViewGoneError();
        const slice = this.items.slice(offset, offset + limit);
        const rows: ViewWindow['rows'] = {};
        for (const it of slice) {
            if (have === 0 || it.seq > have || !this.lastServed.has(it.uid)) {
                rows[it.uid] = {
                    seq: it.seq, ns: it.ns, name: it.name, ts: '', del: false,
                    c: it.cells ?? this.columns.map(c => (c === 'name' ? it.name : `${c}-${it.uid}`)),
                };
            }
        }
        this.lastServed = new Set(slice.map(i => i.uid));
        return { version: this.version, total: this.items.length, filtered: this.items.length, offset, order: slice.map(i => i.uid), rows, state: this.state() };
    }

    async refs(_id: string, offset: number, limit: number, _signal?: AbortSignal, version?: number): Promise<RowRef[]> {
        this.refsCalls.push({ offset, limit, version });
        if (this.staleRefs) {
            this.staleRefs = false;
            throw new ViewStaleError();
        }
        const end = limit > 0 ? offset + limit : this.items.length;
        return this.items.slice(offset, end).map(i => ({ uid: i.uid, ns: i.ns, name: i.name }));
    }

    async facet(_id: string, column: string) {
        return { column, values: ['a'], counts: { a: 1 }, suppressed: false };
    }

    async setSelection(_id: string, uids: string[]) {
        this.selectionCalls.push(uids);
        return { selected: uids.length, stale: this.selectionStale };
    }

    async getObject(ref: { name: string }) {
        return { metadata: { name: ref.name }, spec: { full: true } };
    }

    async getObjects(refs: Array<{ name: string }>) {
        return refs.map(r => ({ metadata: { name: r.name }, spec: { full: true } }));
    }
}

export class FakeFeed implements NoticeFeed {
    private noticeCbs = new Set<(n: ViewNotice[]) => void>();
    private reconnectCbs = new Set<() => void>();

    subscribe(cb: (n: ViewNotice[]) => void): () => void {
        this.noticeCbs.add(cb);
        return () => { this.noticeCbs.delete(cb); };
    }

    onReconnect(cb: () => void): () => void {
        this.reconnectCbs.add(cb);
        return () => { this.reconnectCbs.delete(cb); };
    }

    emit(...notices: ViewNotice[]): void {
        for (const cb of [...this.noticeCbs]) cb(notices);
    }

    reconnect(): void {
        for (const cb of [...this.reconnectCbs]) cb();
    }

    get subscribers(): number {
        return this.noticeCbs.size + this.reconnectCbs.size;
    }
}

export function baseSpec(over: Partial<ViewSpec> = {}): ViewSpec {
    return {
        viewId: 'v1', clientToken: 't', type: 'configmaps', namespaces: ['*'], search: '', columnFilters: [],
        sort: { key: 'name', dir: 'asc' }, columns: [], locale: 'en', hide: [], ...over,
    };
}

export function notice(over: Partial<ViewNotice> = {}): ViewNotice {
    return { v: 'v1', version: 0, total: 0, filtered: 0, phase: 'complete', loaded: 0, expected: 0, ...over };
}

/** Lets pending promise chains run under fake timers. */
export async function flush(times = 10): Promise<void> {
    for (let i = 0; i < times; i++) await vi.advanceTimersByTimeAsync(0);
}
