import React from 'react';
import { describe, expect, it } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import StoreViewsSection from './StoreViewsSection';
import { computeViewRates, shardFlags, type ShardStat, type ViewStat } from './storeViewsStats';

const shard = (over: Partial<ShardStat> = {}): ShardStat => ({
    key: 'c|pods|', type: 'pods', namespace: '', phase: 'watching', mode: 'full', requested: 'full', degraded: false,
    degradedReason: '', entries: 120, estimatedBytes: 2 * 1024 * 1024, refs: 1, lingering: false, subscribers: 1,
    loaded: 120, expected: 120, error: '', forbidden: false, ...over,
});

const view = (over: Partial<ViewStat> = {}): ViewStat => ({
    id: 'v1', type: 'crd:cert.io/v1/certs', namespaces: 1, phase: 'complete', version: 12, total: 50, filtered: 20,
    windowRows: 30, active: true, windows: 4, rowsServed: 100, notices: 10, bytesSent: 10000, idleSecs: 1, ...over,
});

describe('computeViewRates', () => {
    it('derives per-minute rates from cumulative counters', () => {
        const prev = { timestamp: 1000, views: [view({ notices: 10, bytesSent: 10000 })] };
        const cur = { timestamp: 31000, views: [view({ notices: 15, bytesSent: 40000 })] };
        const r = computeViewRates(prev, cur).get('v1')!;
        expect(r.noticesPerMin).toBeCloseTo(10);
        expect(r.bytesPerMin).toBeCloseTo(60000);
    });

    it('has no rate for new views, missing readings or a clock that did not advance', () => {
        const cur = { timestamp: 2000, views: [view({ id: 'new' })] };
        expect(computeViewRates({ timestamp: 1000, views: [view()] }, cur).size).toBe(0);
        expect(computeViewRates(null, cur).size).toBe(0);
        expect(computeViewRates({ timestamp: 2000, views: [view({ id: 'new' })] }, cur).size).toBe(0);
    });

    it('never reports a negative rate when a counter restarted', () => {
        const r = computeViewRates(
            { timestamp: 0, views: [view({ notices: 100 })] },
            { timestamp: 60000, views: [view({ notices: 3 })] },
        ).get('v1')!;
        expect(r.noticesPerMin).toBe(0);
    });
});

describe('shardFlags', () => {
    it('lists degraded, lingering, forbidden and error states', () => {
        expect(shardFlags(shard())).toEqual([]);
        expect(shardFlags(shard({ degraded: true, degradedReason: 'no REST client', lingering: true, forbidden: true, error: 'boom' })))
            .toEqual(['degraded: no REST client', 'lingering', 'forbidden', 'error: boom']);
    });
});

describe('StoreViewsSection', () => {
    it('shows shards with entries, estimated bytes, phase, mode, degraded flag and lingering', () => {
        const html = renderToStaticMarkup(
            <StoreViewsSection metrics={{
                timestamp: 1,
                store: {
                    shards: [shard(), shard({ key: 'c|secrets|', type: 'secrets', mode: 'full', requested: 'table', degraded: true, degradedReason: 'no REST client', lingering: true, refs: 0, entries: 7 })],
                    entries: 127, estimatedBytes: 3 * 1024 * 1024, maxBytes: 1024 * 1024 * 1024,
                },
                views: [],
            }} />,
        );
        expect(html).toContain('2 shards, 127 entries');
        expect(html).toContain('3 MB estimated of 1 GB budget');
        expect(html).toContain('full (asked table)');
        expect(html).toContain('degraded: no REST client; lingering');
        expect(html).toContain('No views are open.');
        expect(html).toContain('only while its list is on screen');
    });

    it('shows per-view counts, window size, version and rates', () => {
        const prev = { timestamp: 0, views: [view({ notices: 0, bytesSent: 0 })] };
        const html = renderToStaticMarkup(
            <StoreViewsSection previous={prev} metrics={{ timestamp: 60000, views: [view({ notices: 6, bytesSent: 2048 })] }} />,
        );
        expect(html).toContain('crd:cert.io/v1/certs');
        expect(html).toContain('20 of 50');
        expect(html).toContain('>12<');
        expect(html).toContain('6.0');
        expect(html).toContain('2 KB');
        expect(html).toContain('The store is not running.');
    });
});
