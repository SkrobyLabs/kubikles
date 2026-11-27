import { describe, expect, it } from 'vitest';
import fixtures from './__fixtures__/protocol.json';
import { isViewGone, isViewWindow, ViewGoneError, type Facet, type ViewNotice, type ViewOpenResult, type ViewWindow } from './protocol';
import { makeRowItem } from './rowItem';
import { getRowSchema } from './schema';

// The fixtures were captured from a running headless debugcluster server (open, window, refs,
// facet, selection, notices, 410). They pin the wire shapes the Go side produces today.

describe('protocol fixtures', () => {
    const open = fixtures.open as unknown as ViewOpenResult;
    const window = fixtures.window as unknown as ViewWindow;

    it('the window is a valid ViewWindow', () => {
        expect(isViewWindow(window)).toBe(true);
        expect(isViewWindow({ version: 1 })).toBe(false);
        expect(isViewWindow(null)).toBe(false);
    });

    it('every row carries cells aligned with the open result columns', () => {
        for (const uid of window.order) {
            const row = window.rows[uid];
            expect(row.c).toHaveLength(open.columns.length);
            expect(typeof row.seq).toBe('number');
            expect(typeof row.del).toBe('boolean');
        }
    });

    it('open columns equal the generated schema column order', () => {
        const schema = getRowSchema('events')!;
        expect(open.columns).toEqual(schema.columns.map(c => c.key));
        expect(open.schemaVersion).toBe(schema.schemaVersion);
    });

    it('a repeat window with have suppresses known rows but keeps the order', () => {
        const again = fixtures.windowHave as unknown as ViewWindow;
        expect(again.order).toEqual(window.order);
        expect(Object.keys(again.rows)).toHaveLength(0);
    });

    it('row items are built from wire rows and keyed by cell name', () => {
        const uid = window.order[0];
        const item = makeRowItem(uid, window.rows[uid], open.columns);
        expect(item.metadata).toMatchObject({ uid, name: 'event-0', namespace: 'debug-ns-0' });
        expect(item.cells.type).toBe('Normal');
        expect(item.cells.involvedObject).toBe('Pod/pod-0');
        expect(item.__seq).toBe(window.rows[uid].seq);
    });

    it('row items keep identity while the seq is unchanged and change with it', () => {
        const uid = window.order[0];
        const wire = window.rows[uid];
        const first = makeRowItem(uid, wire, open.columns);
        expect(makeRowItem(uid, { ...wire }, open.columns, first)).toBe(first);
        const changed = makeRowItem(uid, { ...wire, seq: wire.seq + 1 }, open.columns, first);
        expect(changed).not.toBe(first);
        expect(changed.cells).toEqual(first.cells);
    });

    it('the NDJSON refs stream is a header line then [uid, ns, name] lines', () => {
        const [header, ...lines] = fixtures.refsNdjson.map(l => JSON.parse(l));
        expect(header).toMatchObject({ version: expect.any(Number), total: 50, filtered: 50 });
        expect(lines[0]).toEqual([window.order[0], 'debug-ns-0', 'event-0']);
    });

    it('the facet lists predefined values with counts', () => {
        const facet = fixtures.facet as unknown as Facet;
        expect(facet.values).toEqual(['Normal', 'Warning']);
        expect(facet.counts.Warning).toBeGreaterThan(0);
        expect(facet.suppressed).toBe(false);
    });

    it('notices carry the view id, counts, phase and the windowChanged flag', () => {
        const notice = fixtures.notices[0] as unknown as ViewNotice;
        expect(notice).toMatchObject({ v: 'fx1', phase: 'complete', windowChanged: true });
    });

    it('selection acks list stale uids', () => {
        expect(fixtures.selection.stale).toEqual(['nope']);
    });

    it('an unknown view is 410 with the view-not-found message', () => {
        expect(fixtures.gone.status).toBe(410);
        expect(isViewGone(new Error(fixtures.gone.body.error))).toBe(true);
    });
});

describe('isViewGone', () => {
    it('recognises the status, the error class and both binding error shapes', () => {
        expect(isViewGone(new ViewGoneError())).toBe(true);
        expect(isViewGone(Object.assign(new Error('x'), { status: 410 }))).toBe(true);
        expect(isViewGone('view not found')).toBe(true);
        expect(isViewGone(new Error('View not found'))).toBe(true);
        expect(isViewGone(new Error('boom'))).toBe(false);
        expect(isViewGone(null)).toBe(false);
    });
});
