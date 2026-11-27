import { useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from 'react';
import { useK8s, useUI } from '../context';
import { namespaceSelection } from '../utils/namespaceQuery';
import { useWindowVisible } from '../utils/windowVisibility';
import { getClientId } from '../lib/wailsjs-adapter/runtime/runtime';
import { createViewClient, type ViewHandle, type ViewSnapshot } from '../lib/views/viewClient';
import { createViewHandle } from '../lib/views/viewClientFactory';
import { createDefaultTransport, getNoticeFeed, type NoticeFeed, type ViewTransport } from '../lib/views/transport';
import { getRowSchema } from '../lib/views/schema';
import { markKindDegraded } from '../lib/views/availability';
import { holdsQuery, parkKey, parkView, peekParkedView, takeParkedView, type ParkedQuery } from '../lib/views/viewParking';
import type { ColumnFilterSpec, CrdDescriptor, ViewSpec } from '../lib/views/protocol';
import type { LoadingProgress, ResourceLoadState } from './useResource';

// One token per page load groups this client's views for caps and lease reaping on the server.
const CLIENT_TOKEN = (typeof crypto !== 'undefined' && 'randomUUID' in crypto)
    ? crypto.randomUUID()
    : `c-${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

const CONTEXT_REOPEN_DELAY_MS = 300;

let idCounter = 0;
export function newViewId(): string {
    const rand = (typeof crypto !== 'undefined' && 'randomUUID' in crypto)
        ? crypto.randomUUID()
        : `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
    return `v-${++idCounter}-${rand}`;
}

export interface UseResourceViewOptions {
    /** Open the view only when true (setting on, type migrated, list visible, context known). */
    enabled: boolean;
    context: string | null;
    namespaces: string | string[] | null | undefined;
    sort?: { key: string | null; direction: string };
    /** Named presets that remove rows from the scope ("helm", "system"). */
    hide?: string[];
    search?: string;
    columnFilters?: ColumnFilterSpec[];
    columns?: string[];
    /** Event kinds the view reports in notices besides counts ("pods" on the pod list). */
    notifyOn?: string[];
    /** Custom resource descriptor ("crd:" types): scope and identity for the backend's dynamic spec. */
    crd?: CrdDescriptor;
    // Injection points for tests.
    transport?: ViewTransport;
    notices?: NoticeFeed;
}

export interface UseResourceViewResult {
    view: ViewHandle | null;
    snapshot: ViewSnapshot | null;
    total: number;
    filtered: number;
    loading: boolean;
    error: Error | null;
    loadingProgress: LoadingProgress | null;
    loadState: ResourceLoadState;
    refetch: () => void;
}

const EMPTY_SNAPSHOT_SUBSCRIBE = () => () => {};
const EMPTY_SNAPSHOT = () => null;

function toSort(sort: UseResourceViewOptions['sort']): { key: string; dir: 'asc' | 'desc' | '' } {
    if (!sort || !sort.key) return { key: '', dir: '' };
    return { key: sort.key, dir: sort.direction === 'desc' ? 'desc' : 'asc' };
}

export function useResourceView(resourceType: string, opts: UseResourceViewOptions): UseResourceViewResult {
    const { lastRefresh, reconcileToken, connectionMode, registerPolling, checkConnectionError } = useK8s();
    const visible = useWindowVisible();
    const schema = getRowSchema(resourceType);
    const namespaced = opts.crd?.namespaced ?? schema?.namespaced ?? true;

    const nsKey = useMemo(
        () => JSON.stringify(namespaced ? namespaceSelection(opts.namespaces) : ['*']),
        [namespaced, opts.namespaces],
    );
    const hideKey = JSON.stringify(opts.hide ?? []);
    const sortKey = JSON.stringify(toSort(opts.sort));
    const searchKey = opts.search ?? '';
    const filtersKey = JSON.stringify(opts.columnFilters ?? []);
    const columnsKey = JSON.stringify(opts.columns ?? []);
    const notifyKey = JSON.stringify(opts.notifyOn ?? []);

    // The latest query, read when the view opens; later changes go through update().
    const latest = useRef({ nsKey, hideKey, sortKey, searchKey, filtersKey, columnsKey, notifyKey });
    latest.current = { nsKey, hideKey, sortKey, searchKey, filtersKey, columnsKey, notifyKey };

    const crdRef = useRef(opts.crd);
    crdRef.current = opts.crd;
    const transportRef = useRef(opts.transport);
    transportRef.current = opts.transport;
    const noticesRef = useRef(opts.notices);
    noticesRef.current = opts.notices;

    const open = opts.enabled && !!opts.context;
    // A list left a moment ago still has its view parked: start from that handle so the first
    // render already has rows. The effect below takes ownership of it.
    // It is reused only when it holds exactly the query a fresh open would send, and never when
    // the list was opened by a link that carries a search: a reused view paints its rows at
    // once, so rows of another query would be shown, and acted on, as if they were current.
    // Latched: the list consumes the pending search in its own mount effect, which can run
    // before this hook's open effect does (the hook may first render while still disabled).
    const { pendingSearch } = useUI();
    const linkedOpenRef = useRef(false);
    if (pendingSearch) linkedOpenRef.current = true;
    const linkedOpen = linkedOpenRef.current;
    const wantedQuery = (): ParkedQuery => ({
        namespaces: JSON.parse(nsKey), hide: JSON.parse(hideKey), sort: JSON.parse(sortKey), search: searchKey,
    });
    const reusable = (handle: ViewHandle | null): ViewHandle | null => (
        handle && !linkedOpen && holdsQuery(handle, wantedQuery()) ? handle : null);
    const [view, setView] = useState<ViewHandle | null>(() => (
        open && !opts.transport && !opts.notices ? reusable(peekParkedView(parkKey(opts.context!, resourceType))) : null));

    useEffect(() => {
        if (!open) return;
        // Injected transport or notices (tests) run the engine inline and are never parked;
        // otherwise the factory picks worker, main-thread-fetch or inline by what this WebView supports.
        const injected = !!(transportRef.current || noticesRef.current);
        const key = parkKey(opts.context!, resourceType);
        const parked = injected ? null : takeParkedView(key);
        const reused = reusable(parked);
        // A parked view that does not fit is closed; the list opens a fresh one below.
        if (parked && !reused) parked.close();
        if (reused) {
            // The effects below bring it up to date: scope patches, active state, and the list's
            // own refresh. Patches equal to the retained spec send nothing.
            setView(reused);
            return () => {
                parkView(key, reused);
                setView(current => (current === reused ? null : current));
            };
        }
        const handle = injected
            ? createViewClient({
                transport: transportRef.current ?? createDefaultTransport(),
                notices: noticesRef.current ?? getNoticeFeed(),
            })
            : createViewHandle();
        const q = latest.current;
        const spec: ViewSpec = {
            viewId: newViewId(),
            clientToken: CLIENT_TOKEN,
            clientId: getClientId() || '',
            type: resourceType,
            namespaces: JSON.parse(q.nsKey),
            search: q.searchKey,
            columnFilters: JSON.parse(q.filtersKey),
            sort: JSON.parse(q.sortKey),
            columns: JSON.parse(q.columnsKey),
            locale: typeof navigator !== 'undefined' ? navigator.language : '',
            hide: JSON.parse(q.hideKey),
            ...(q.notifyKey !== '[]' ? { notifyOn: JSON.parse(q.notifyKey) } : {}),
            ...(crdRef.current ? { crd: crdRef.current } : {}),
        };
        setView(handle);
        void handle.open(spec);
        return () => {
            if (injected) handle.close(); else parkView(key, handle);
            setView(current => (current === handle ? null : current));
        };
    }, [open, opts.context, resourceType]);

    // Scope changes after open travel as patches (the client ignores no-op patches). Search,
    // column filters, sort and columns belong to the list component, which pushes them through
    // view.update(); the values passed here only seed a freshly opened view.
    useEffect(() => {
        if (!view) return;
        view.update({ namespaces: JSON.parse(nsKey), hide: JSON.parse(hideKey) });
    }, [view, nsKey, hideKey]);
    // Turning an event kind on or off after open is a patch too; a view already in the wanted state sends nothing.
    useEffect(() => {
        if (!view) return;
        const want: string[] = JSON.parse(notifyKey);
        const have = view.spec()?.notifyOn ?? [];
        if (JSON.stringify(want) === JSON.stringify(have)) return;
        view.update({ notifyOn: want });
    }, [view, notifyKey]);
    // Hidden window: the backend stops notifying for this view and catches up when shown.
    useEffect(() => {
        if (!view) return;
        view.setActive(visible);
    }, [view, visible]);

    // Explicit refresh and reconciliation refetch the window; the store keeps the data live.
    const refreshMark = useRef({ lastRefresh, reconcileToken });
    useEffect(() => {
        const prev = refreshMark.current;
        refreshMark.current = { lastRefresh, reconcileToken };
        if (!view) return;
        if (prev.lastRefresh !== lastRefresh || prev.reconcileToken !== reconcileToken) view.refresh();
    }, [view, lastRefresh, reconcileToken]);

    useEffect(() => {
        if (!view || connectionMode !== 'polling') return;
        return registerPolling(async () => { view.refresh(); });
    }, [view, connectionMode, registerPolling]);

    const snapshot = useSyncExternalStore(
        view ? view.subscribe : EMPTY_SNAPSHOT_SUBSCRIBE,
        view ? view.getSnapshot : EMPTY_SNAPSHOT,
        view ? view.getSnapshot : EMPTY_SNAPSHOT,
    ) as ViewSnapshot | null;

    // The backend closes every view when its context changes. A UI context switch unmounts the
    // handle first (the open effect is keyed on the context), so reaching this with the handle still
    // current means the backend changed underneath the UI (restart, another window): open again.
    useEffect(() => {
        if (!view || snapshot?.status !== 'closed' || snapshot.closedReason !== 'context') return;
        const timer = setTimeout(() => view.reopen(), CONTEXT_REOPEN_DELAY_MS);
        return () => clearTimeout(timer);
    }, [view, snapshot?.status, snapshot?.closedReason]);

    // Repeated failure marks the kind legacy for the session; the wrapper then mounts the old hook.
    useEffect(() => {
        if (snapshot?.permanent) markKindDegraded(resourceType);
    }, [snapshot?.permanent, resourceType]);

    const errorMessage = snapshot?.status === 'failed' ? snapshot.error : snapshot?.state?.error || null;
    const error = useMemo(() => (errorMessage ? new Error(errorMessage) : null), [errorMessage]);
    useEffect(() => {
        if (error) checkConnectionError(error);
    }, [error, checkConnectionError]);

    const refetch = useCallback(() => { view?.reopen(); }, [view]);

    const st = snapshot?.state ?? null;
    const rawPhase: ResourceLoadState['phase'] = !snapshot || snapshot.status === 'opening' || snapshot.status === 'idle'
        ? 'loading'
        : snapshot.status === 'failed' ? 'incomplete' : (st?.phase ?? 'loading');
    // A first load that is failing (the backend keeps retrying) still reports "loading". Shown
    // as that, the list would sit on "Loading..." with the error never displayed; the legacy
    // list reports any error as incomplete, which shows the reason and a retry.
    const phase: ResourceLoadState['phase'] = rawPhase === 'loading' && st?.error ? 'incomplete' : rawPhase;
    const loadingProgress = useMemo<LoadingProgress | null>(
        () => (st && st.expected > 0 && (phase === 'loading' || phase === 'refreshing')
            ? { loaded: st.loaded, total: st.expected } : null),
        [st?.loaded, st?.expected, phase],
    );
    const loadState = useMemo<ResourceLoadState>(
        () => ({ phase, error, progress: loadingProgress, retry: refetch }),
        [phase, error, loadingProgress, refetch],
    );

    return {
        view,
        snapshot,
        total: snapshot?.total ?? 0,
        filtered: snapshot?.filtered ?? 0,
        loading: phase === 'loading' || phase === 'refreshing',
        error,
        loadingProgress,
        loadState,
        refetch,
    };
}
