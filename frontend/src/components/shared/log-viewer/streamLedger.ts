/**
 * Remembers which log streams this page opened, across a reload.
 *
 * A stream is stopped by the viewer that started it. A page reload in the desktop app removes the
 * viewers without running their cleanup, so their streams would keep reading from the cluster
 * until the app quits. The ids are therefore kept in sessionStorage, which survives a reload of
 * the same window, and the next page stops whatever the previous one left behind before it opens
 * any stream of its own.
 *
 * Desktop only: a browser copies sessionStorage into a duplicated tab, and stopping "leftovers"
 * there would cut the streams of the tab it was copied from. Server mode has its own cleanup
 * once no client is connected.
 */
const KEY = 'kubikles_log_streams';
const MAX_REMEMBERED = 64;

type Store = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

const isDesktop = () => typeof window !== 'undefined' && typeof (window as any).runtime !== 'undefined';
const sessionStore = (): Store | null => {
    try { return isDesktop() ? window.sessionStorage : null; } catch { return null; }
};

function read(store: Store): string[] {
    try {
        const ids = JSON.parse(store.getItem(KEY) ?? '[]');
        return Array.isArray(ids) ? ids.filter(id => typeof id === 'string') : [];
    } catch {
        return [];
    }
}

function write(store: Store, ids: string[]): void {
    try {
        if (ids.length === 0) store.removeItem(KEY);
        else store.setItem(KEY, JSON.stringify(ids.slice(-MAX_REMEMBERED)));
    } catch { /* storage unavailable: the leak is back, nothing else breaks */ }
}

export function rememberStream(id: string, store: Store | null = sessionStore()): void {
    if (!store || !id) return;
    const ids = read(store);
    if (!ids.includes(id)) write(store, [...ids, id]);
}

export function forgetStream(id: string, store: Store | null = sessionStore()): void {
    if (!store || !id) return;
    const ids = read(store);
    if (ids.includes(id)) write(store, ids.filter(other => other !== id));
}

/** Stops every stream a previous page of this window left running. Stopping one that already ended does nothing. */
export function stopLeftoverStreams(stop: (id: string) => void, store: Store | null = sessionStore()): number {
    if (!store) return 0;
    const ids = read(store);
    write(store, []);
    for (const id of ids) {
        try { stop(id); } catch { /* best effort */ }
    }
    return ids.length;
}
