// Helpers shared by the view-column tests (pilot and batch lists): rendering rows without a DOM
// and proving that column renderers read nothing but the cells the schema provides.

import type { RowItem } from '~/lib/views/rowItem';

/** Text content of a rendered React node tree (no DOM needed). */
export const text = (node: any): string => {
    if (node == null || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(text).join('');
    return text(node.props?.children);
};

export const byKey = (cols: any[], key: string) => cols.find(c => c.key === key)!;

/** A plain windowed row. */
export const rowItem = (cells: Record<string, any>, over: Partial<RowItem['metadata']> = {}): RowItem => ({
    metadata: { uid: 'u1', name: 'thing', namespace: 'ns', ...over }, cells, __seq: 1, __view: true,
});

/** Row whose cells and metadata record every key read by render code. */
export function recordingItem(cells: Record<string, any>) {
    const reads = new Set<string>();
    const wrap = (obj: any, prefix: string) => new Proxy(obj, {
        get(target, prop, receiver) {
            if (typeof prop === 'string') reads.add(`${prefix}.${prop}`);
            return Reflect.get(target, prop, receiver);
        },
    });
    const item = {
        metadata: wrap({ uid: 'u1', name: 'thing', namespace: 'ns' }, 'metadata'),
        cells: wrap(cells, 'cells'),
        __seq: 1,
        __view: true,
    } as unknown as RowItem;
    return { item, reads };
}
