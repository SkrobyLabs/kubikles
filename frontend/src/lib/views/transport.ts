import {
    OpenView, UpdateView, CloseView, SetViewActive, GetViewWindow, GetViewRefs, GetViewFacet,
    SetViewSelection, GetResourceObject, GetResourceObjects, GetViewsInfo,
} from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { getClientId } from '../wailsjs-adapter/runtime/runtime';
import {
    BulkUnavailableError, bulkFetchJson, bulkFetchNdjson, bulkFetchUsable, noteBulkResult,
} from '../bulk/transport';
import {
    ViewGoneError, ViewStaleError, isViewGone, isViewStale, isViewWindow,
    type Facet, type ObjectRef, type RowRef, type SelectionAck, type StreamHeader, type ViewNotice,
    type ViewOpenResult, type ViewPatch, type ViewSpec, type ViewState, type ViewWindow, type ViewsInfo,
} from './protocol';

/**
 * Everything the view client needs from the outside world. Control goes over bindings, data over
 * the GET routes with the bindings as fallback. The client only sees this interface, so it can
 * run unchanged on the main thread or inside a worker with another implementation.
 */
export interface ViewTransport {
    open(spec: ViewSpec): Promise<ViewOpenResult>;
    update(viewId: string, patch: ViewPatch): Promise<ViewState>;
    close(viewId: string): Promise<void>;
    setActive(viewId: string, active: boolean): Promise<void>;
    /** Rejects with a view-gone error (see isViewGone) when the view does not exist. */
    window(viewId: string, offset: number, limit: number, have: number, signal?: AbortSignal): Promise<ViewWindow>;
    /**
     * `version` is the view version the offsets were read at; the call rejects with a stale error
     * (see isViewStale) when the list has moved on. Zero skips the check.
     */
    refs(viewId: string, offset: number, limit: number, signal?: AbortSignal, version?: number): Promise<RowRef[]>;
    facet(viewId: string, column: string): Promise<Facet>;
    setSelection(viewId: string, uids: string[]): Promise<SelectionAck>;
    getObject(ref: ObjectRef): Promise<Record<string, any>>;
    getObjects(refs: ObjectRef[]): Promise<Array<Record<string, any>>>;
}

export interface NoticeFeed {
    /** Notices of all views, batched. Returns the unsubscribe function. */
    subscribe(cb: (notices: ViewNotice[]) => void): () => void;
    /** Fires when the event channel reconnected and notices may have been lost. */
    onReconnect(cb: () => void): () => void;
}

const OBJECT_FETCH_CONCURRENCY = 6;

function objectPath(ref: ObjectRef): string {
    const q = new URLSearchParams({ ns: ref.namespace ?? '', name: ref.name });
    if (ref.uid) q.set('uid', ref.uid);
    return `objects/${encodeURIComponent(ref.type)}?${q.toString()}`;
}

/**
 * Statuses that mean the route itself is unreachable (gateway failures). Any other response, a
 * 404 for a deleted object or a 500 with an {error} body for an RBAC refusal, shows the route
 * answered and must not count towards degrading the bulk transport for the session.
 */
function isTransportFailure(e: BulkUnavailableError): boolean {
    const s = e.status;
    return s === undefined || s === 0 || s === 502 || s === 503 || s === 504;
}

/**
 * Runs the bulk attempt; a transport failure (not a view-gone) reports false so the caller falls
 * back. A request the caller aborted is neither a success nor a failure of the transport.
 */
async function viaBulk<T>(run: () => Promise<T>, signal?: AbortSignal): Promise<{ ok: true; value: T } | { ok: false }> {
    if (!(await bulkFetchUsable())) return { ok: false };
    try {
        const value = await run();
        noteBulkResult(true);
        return { ok: true, value };
    } catch (e) {
        if (isViewGone(e)) throw new ViewGoneError();
        if (isViewStale(e)) throw new ViewStaleError();
        if (signal?.aborted) throw e;
        if (e instanceof BulkUnavailableError) {
            noteBulkResult(!isTransportFailure(e), e.message);
            return { ok: false };
        }
        throw e;
    }
}

function normalizeGone(e: unknown): never {
    if (isViewGone(e)) throw new ViewGoneError();
    throw e;
}

async function fetchObject(ref: ObjectRef): Promise<Record<string, any>> {
    const bulk = await viaBulk(() => bulkFetchJson<Record<string, any>>(objectPath(ref)));
    if (bulk.ok) return bulk.value;
    return (await GetResourceObject(ref as any)) as Record<string, any>;
}

export function createDefaultTransport(): ViewTransport {
    return {
        // The owner id is re-read on every open: after a websocket reconnect the server knows the
        // client under a new id, and a retained spec still carries the old one.
        open: spec => OpenView({ ...spec, clientId: getClientId() || spec.clientId || '' } as any) as Promise<ViewOpenResult>,
        update: async (id, patch) => {
            try {
                return (await UpdateView(id, patch as any)) as ViewState;
            } catch (e) {
                return normalizeGone(e);
            }
        },
        close: async id => { await CloseView(id); },
        setActive: async (id, active) => { await SetViewActive(id, active); },

        async window(id, offset, limit, have, signal) {
            const q = `offset=${offset}&limit=${limit}&have=${have}`;
            const bulk = await viaBulk(async () => {
                const w = await bulkFetchJson<ViewWindow>(`views/${encodeURIComponent(id)}/window?${q}`, signal);
                if (!isViewWindow(w)) throw new BulkUnavailableError('malformed window');
                return w;
            }, signal);
            if (bulk.ok) return bulk.value;
            try {
                return (await GetViewWindow(id, offset, limit, have)) as ViewWindow;
            } catch (e) {
                return normalizeGone(e);
            }
        },

        async refs(id, offset, limit, signal, version) {
            const bulk = await viaBulk(async () => {
                const out: RowRef[] = [];
                let first = true;
                const v = version ? `&version=${version}` : '';
                for await (const line of bulkFetchNdjson<unknown>(
                    `views/${encodeURIComponent(id)}/refs?offset=${offset}&limit=${limit}${v}`, signal)) {
                    if (first) { first = false; void (line as StreamHeader); continue; }
                    const [uid, ns, name] = line as [string, string, string];
                    out.push({ uid, ns, name });
                }
                return out;
            }, signal);
            if (bulk.ok) return bulk.value;
            try {
                return ((await GetViewRefs(id, offset, limit, version ?? 0)) as RowRef[]) ?? [];
            } catch (e) {
                if (isViewStale(e)) throw new ViewStaleError();
                return normalizeGone(e);
            }
        },

        async facet(id, column) {
            try {
                return (await GetViewFacet(id, column)) as Facet;
            } catch (e) {
                return normalizeGone(e);
            }
        },

        async setSelection(id, uids) {
            try {
                return (await SetViewSelection(id, uids)) as SelectionAck;
            } catch (e) {
                return normalizeGone(e);
            }
        },

        getObject: fetchObject,

        async getObjects(refs) {
            if (refs.length === 0) return [];
            if (!(await bulkFetchUsable())) return (await GetResourceObjects(refs as any)) as Array<Record<string, any>>;
            const out: Array<Record<string, any>> = new Array(refs.length);
            let next = 0;
            const worker = async () => {
                for (;;) {
                    const i = next++;
                    if (i >= refs.length) return;
                    out[i] = await fetchObject(refs[i]);
                }
            };
            await Promise.all(Array.from({ length: Math.min(OBJECT_FETCH_CONCURRENCY, refs.length) }, worker));
            return out;
        },
    };
}

let sharedFeed: NoticeFeed | null = null;

/** The single EventsOn subscription for notices, shared by every view in the process. */
export function getNoticeFeed(): NoticeFeed {
    if (sharedFeed) return sharedFeed;
    const noticeCbs = new Set<(n: ViewNotice[]) => void>();
    const reconnectCbs = new Set<() => void>();
    let offNotices: (() => void) | null = null;
    let offReconnect: (() => void) | null = null;

    const attach = () => {
        if (!offNotices) {
            offNotices = EventsOn('view-updates', (payload: unknown) => {
                const list: ViewNotice[] = Array.isArray(payload) ? payload as ViewNotice[]
                    : payload && typeof (payload as ViewNotice).v === 'string' ? [payload as ViewNotice] : [];
                if (list.length === 0) return;
                for (const cb of [...noticeCbs]) cb(list);
            });
        }
        if (!offReconnect) {
            offReconnect = EventsOn('ws:reconnected', () => { for (const cb of [...reconnectCbs]) cb(); });
        }
    };
    const detach = () => {
        if (noticeCbs.size === 0 && reconnectCbs.size === 0) {
            offNotices?.(); offNotices = null;
            offReconnect?.(); offReconnect = null;
        }
    };
    sharedFeed = {
        subscribe(cb) {
            noticeCbs.add(cb); attach();
            return () => { noticeCbs.delete(cb); detach(); };
        },
        onReconnect(cb) {
            reconnectCbs.add(cb); attach();
            return () => { reconnectCbs.delete(cb); detach(); };
        },
    };
    return sharedFeed;
}

export async function loadViewsInfo(): Promise<ViewsInfo> {
    const info = (await GetViewsInfo()) as ViewsInfo;
    return { enabled: !!info?.enabled, types: Array.isArray(info?.types) ? info.types : [], dynamic: info?.dynamic === true };
}
