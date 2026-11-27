import type { ViewHandle } from './viewClient';
import type { ViewSpec } from './protocol';

/**
 * Keeps the view of a list the user just left open for a short while. Coming back to the list
 * within that time reuses the handle with its rows, counts and server-side view intact, so the
 * list paints at once and then catches up, instead of opening a new view and refilling.
 *
 * A parked view is inactive (the backend sends it no notices). It is closed when its time is up,
 * when a newer view of the same list is parked, or when too many are held.
 */
export const PARK_MS = 60_000;
export const PARK_MAX = 8;

interface Parked { handle: ViewHandle; timer: ReturnType<typeof setTimeout> }

const parked = new Map<string, Parked>();

export function parkKey(context: string, resourceType: string): string {
    return `${context}\u0000${resourceType}`;
}

function drop(key: string): void {
    const entry = parked.get(key);
    if (!entry) return;
    clearTimeout(entry.timer);
    parked.delete(key);
    entry.handle.close();
}

/** Only a view that is open can be reused; one the backend closed or that failed is opened afresh. */
function reusable(handle: ViewHandle): boolean {
    return handle.getSnapshot().status === 'open';
}

export function parkView(key: string, handle: ViewHandle): void {
    drop(key);
    if (!reusable(handle)) { handle.close(); return; }
    handle.setActive(false);
    // Map order is insertion order: the first key is the one parked longest ago.
    while (parked.size >= PARK_MAX) drop(parked.keys().next().value as string);
    parked.set(key, { handle, timer: setTimeout(() => drop(key), PARK_MS) });
}

/** The parked handle for a list, left in place (for a first render that an effect then confirms). */
export function peekParkedView(key: string): ViewHandle | null {
    const entry = parked.get(key);
    return entry && reusable(entry.handle) ? entry.handle : null;
}

/** Removes and returns the parked handle; the caller owns it from here. */
export function takeParkedView(key: string): ViewHandle | null {
    const entry = parked.get(key);
    if (!entry) return null;
    if (!reusable(entry.handle)) { drop(key); return null; }
    clearTimeout(entry.timer);
    parked.delete(key);
    return entry.handle;
}

/** Closes every parked view (tests, and a full reset). */
export function clearParkedViews(): void {
    for (const key of [...parked.keys()]) drop(key);
}

/** The parts of a query that decide which rows a view holds and in what order. */
export type ParkedQuery = Pick<ViewSpec, 'namespaces' | 'hide' | 'sort' | 'search'>;

/**
 * True when the handle's retained spec is the query a list would open with: same scope, same
 * order, the given search and no column filters. Columns are left out on purpose: a different
 * projection changes which cells a row carries, not which rows are shown.
 */
export function holdsQuery(handle: ViewHandle, want: ParkedQuery): boolean {
    const spec = handle.spec();
    if (!spec) return false;
    const same = (a: unknown, b: unknown) => JSON.stringify(a ?? null) === JSON.stringify(b ?? null);
    return (spec.search ?? '') === (want.search ?? '')
        && (spec.columnFilters ?? []).length === 0
        && same(spec.sort, want.sort)
        && same(spec.namespaces, want.namespaces)
        && same(spec.hide ?? [], want.hide ?? []);
}
