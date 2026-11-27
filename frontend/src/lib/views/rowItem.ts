import type { RowWire } from './protocol';

/**
 * The row object lists render. It is shaped like the legacy items where the list code only
 * needs identity (metadata) and carries the projected cells for everything else. Identity of the
 * object changes only when the row changed, so memoised table rows keep working.
 */
export interface RowItem {
    metadata: {
        uid: string;
        name: string;
        namespace?: string;
        creationTimestamp?: string;
        deletionTimestamp?: string;
    };
    cells: Record<string, any>;
    __seq: number;
    /** Set on rows built from a view; lets list code tell them from legacy items. */
    __view: true;
}

export function isRowItem(x: unknown): x is RowItem {
    return !!x && typeof x === 'object' && (x as RowItem).__view === true;
}

/**
 * Builds RowItems for one column set. build() returns the previous item when its seq is
 * unchanged, which is the memoisation by (uid, seq) the table relies on.
 */
export function makeRowItem(uid: string, wire: RowWire, columns: readonly string[], previous?: RowItem): RowItem {
    if (previous && previous.__seq === wire.seq) return previous;
    const cells: Record<string, any> = {};
    for (let i = 0; i < columns.length; i++) cells[columns[i]] = wire.c[i] ?? null;
    const metadata: RowItem['metadata'] = { uid, name: wire.name };
    if (wire.ns) metadata.namespace = wire.ns;
    if (wire.ts) metadata.creationTimestamp = wire.ts;
    if (wire.del) metadata.deletionTimestamp = 'deleting';
    return { metadata, cells, __seq: wire.seq, __view: true };
}

/** Minimal item handed to bulk actions: identity only. Callers fetch the object for spec. */
export interface RowRefItem {
    metadata: { uid: string; name: string; namespace?: string };
}

export function refItem(uid: string, ns: string, name: string): RowRefItem {
    const metadata: RowRefItem['metadata'] = { uid, name };
    if (ns) metadata.namespace = ns;
    return { metadata };
}
