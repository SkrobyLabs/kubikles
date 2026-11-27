import { useCallback, useEffect, useState } from 'react';
import { useNotification } from '../context';
import type { ViewHandle, ViewSnapshot } from '../lib/views/viewClient';
import { isRowItem } from '../lib/views/rowItem';

/**
 * Rows of a windowed list carry identity and projected cells only. Actions that need the object
 * itself (details tabs, anything reading spec or status) are wrapped with this: at click time it
 * fetches the full object through the view and hands that to the handler. Outside view mode, or
 * for items that are already full objects, the handler is called directly.
 */
export function useWithFullObject(view: ViewHandle | null) {
    const { addNotification } = useNotification();
    return useCallback(<A extends any[]>(handler: (item: any, ...rest: A) => void) => (item: any, ...rest: A): void => {
        if (!view || !isRowItem(item)) {
            handler(item, ...rest);
            return;
        }
        view.getObject(item).then(obj => handler(obj, ...rest)).catch((err: unknown) => {
            addNotification({ type: 'error', title: 'Failed to load resource', message: String((err as Error)?.message ?? err) });
        });
    }, [view, addNotification]);
}

/** The upper bound of the window a whole-list consumer asks for; a cluster has far fewer nodes. */
const ALL_OBJECTS_CAP = 5000;
const NO_OBJECTS: Array<Record<string, any>> = [];

/**
 * Every object of a view, by name. The refs stream lists the whole filtered scope and the object
 * route serves each object from the shared store shard, so no legacy list or watch is needed;
 * sorting by name gives the order of a Kubernetes list response (keys in byte order).
 */
export async function fetchAllViewObjects(view: Pick<ViewHandle, 'selectAllRefs' | 'getObjects'>): Promise<Array<Record<string, any>>> {
    const refs = await view.selectAllRefs();
    if (refs.length === 0) return [];
    const objects = await view.getObjects(refs);
    return [...objects].sort((a, b) => {
        const x = a?.metadata?.name ?? '', y = b?.metadata?.name ?? '';
        return x < y ? -1 : x > y ? 1 : 0;
    });
}

/**
 * The whole object list of a view, for consumers that draw from every object (the node topology
 * diagram) instead of a window of rows. While enabled the hook keeps a window over the whole
 * scope open, which is what makes the backend notify it of changes, and re-reads the objects
 * whenever the view reports a new version. Disabled, it holds nothing and fetches nothing.
 */
export function useAllViewObjects(view: ViewHandle | null, snapshot: ViewSnapshot | null, enabled: boolean): { objects: Array<Record<string, any>>; loading: boolean } {
    const [state, setState] = useState<{ objects: Array<Record<string, any>>; loading: boolean }>({ objects: NO_OBJECTS, loading: true });
    const open = !!view && enabled && snapshot?.status === 'open';
    const filtered = snapshot?.filtered ?? 0;
    const version = snapshot?.state?.version ?? 0;

    useEffect(() => {
        if (!view || !open) return;
        view.setRange(0, Math.min(Math.max(filtered, 1), ALL_OBJECTS_CAP));
    }, [view, open, filtered]);

    useEffect(() => {
        if (!view || !open) return;
        let cancelled = false;
        fetchAllViewObjects(view)
            .then(objects => { if (!cancelled) setState({ objects, loading: false }); })
            .catch(() => { if (!cancelled) setState(prev => ({ ...prev, loading: false })); });
        return () => { cancelled = true; };
    }, [view, open, version, filtered]);

    useEffect(() => {
        if (!enabled) setState(prev => (prev.objects === NO_OBJECTS && prev.loading ? prev : { objects: NO_OBJECTS, loading: true }));
    }, [enabled]);

    return state;
}
