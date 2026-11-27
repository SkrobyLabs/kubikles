import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it, vi } from 'vitest';
import { RowCells, virtuosoComponents } from './ResourceListRows';

const item = { metadata: { name: 'web', namespace: 'prod', uid: 'u1' } };
const columns: any[] = [
    { key: 'name', render: (i: any) => i.metadata.name },
    { key: 'namespace', render: (i: any) => i.metadata.namespace },
];

describe('RowCells', () => {
    it('renders one cell per column with widths, titles and a namespace filter button', () => {
        const html = renderToStaticMarkup(
            <table><tbody><tr>
                <RowCells item={item} index={0} columns={columns} columnWidths={{ name: 200 }} selected={false} onNamespaceChange={() => {}} />
            </tr></tbody></table>,
        );
        expect(html).toContain('title="web"');
        expect(html).toContain('width:200px');
        expect(html).toContain('Filter to namespace: prod');
    });

    it('renders the selection checkbox state', () => {
        const sel = [{ key: '_selection', isSelectionColumn: true, width: 44 }, ...columns];
        const render = (selected: boolean) => renderToStaticMarkup(
            <table><tbody><tr><RowCells item={item} index={0} columns={sel} columnWidths={{}} selected={selected} /></tr></tbody></table>,
        );
        expect(render(true)).toContain('checked');
        expect(render(false)).not.toContain('checked');
    });
});

describe('TableRow memoisation', () => {
    const compare = (virtuosoComponents.TableRow as any).compare as (a: any, b: any) => boolean;
    const context = { selectable: false };
    const widths = {};
    const cells = (props: Record<string, any> = {}) => (
        <RowCells item={item} index={0} columns={columns} columnWidths={widths} selected={false} {...props} />
    );
    const base = () => ({ item, context, style: { height: 20 }, 'data-index': 0, children: cells() });

    it('treats a freshly created but identical <RowCells> child as unchanged', () => {
        const a = base();
        const b = { ...base(), style: a.style };
        expect(a.children).not.toBe(b.children);
        expect(compare(a, b)).toBe(true);
    });

    it('re-renders when the item, context, row props or cell props change', () => {
        const a = base();
        expect(compare(a, { ...a, item: { ...item } })).toBe(false);
        expect(compare(a, { ...a, context: { selectable: true } })).toBe(false);
        expect(compare(a, { ...a, 'data-index': 1 })).toBe(false);
        expect(compare(a, { ...a, children: cells({ selected: true }) })).toBe(false);
        expect(compare(a, { ...a, children: cells({ columnWidths: {} }) })).toBe(false);
        expect(compare(a, { ...a, children: <div /> })).toBe(false);
    });

    it('keeps table component types stable so hover state survives updates', () => {
        const again = (virtuosoComponents as any);
        expect(again.TableRow).toBe(virtuosoComponents.TableRow);
        expect(vi.isMockFunction(again.Table)).toBe(false);
    });
});
