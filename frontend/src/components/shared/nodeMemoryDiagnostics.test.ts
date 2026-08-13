import { describe, expect, it } from 'vitest';
import { alignedValue, contributorSummaries, freshness, minimumPoint, nonNegative, pressureEpisodes, segmentSeries, timelineTimeTicks, timelineTooltip, timelineValueTicks } from './nodeMemoryDiagnostics';

describe('node memory diagnostics helpers', () => {
    it('aligns timestamps without array indexes and keeps residuals non-negative', () => {
        expect(alignedValue([{ timestamp: 1000, value: 3 }], 1100, 500)).toBe(3);
        expect(alignedValue([{ timestamp: 1000, value: 3 }], 1400, 500)).toBeUndefined();
        expect(nonNegative(-3)).toBe(0);
        expect(minimumPoint([{ timestamp: 1, value: 4 }, { timestamp: 2, value: 2 }])?.value).toBe(2);
    });

    it('counts pressure edges, uses the displayed range end for freshness, and splits only larger gaps', () => {
        expect(pressureEpisodes([{ timestamp: 1, value: 0 }, { timestamp: 2, value: 1 }, { timestamp: 3, value: 1 }, { timestamp: 4, value: 0 }, { timestamp: 5, value: 1 }])).toBe(2);
        expect(freshness({ timestamp: 9_000, value: 1 }, 10_000, 500)).toBe('fresh');
        expect(freshness({ timestamp: 1_000, value: 1 }, 10_000, 500)).toBe('stale');
        expect(segmentSeries([{ timestamp: 0, value: 1 }, { timestamp: 200, value: 2 }, { timestamp: 401, value: 3 }], 100)).toHaveLength(2);
    });

    it('ranks contributors deterministically with their latest observation', () => {
        const summaries = contributorSummaries([
            { namespace: 'z', pod: 'a', workingSet: [{ timestamp: 1, value: 2 }] },
            { namespace: 'a', pod: 'z', workingSet: [{ timestamp: 1, value: 2 }, { timestamp: 2, value: 1 }] },
        ]);
        expect(summaries.map(item => `${item.namespace}/${item.pod}`)).toEqual(['a/z', 'z/a']);
        expect(summaries[0].last?.timestamp).toBe(2);
    });

    it('builds readable axis ticks and only shows observed values in a bounded hover tooltip', () => {
        expect(timelineTimeTicks(1_000, 5_000, 3)).toEqual([
            { timestamp: 1_000, ratio: 0 },
            { timestamp: 3_000, ratio: 0.5 },
            { timestamp: 5_000, ratio: 1 },
        ]);
        expect(timelineValueTicks(100, 3)).toEqual([
            { value: 100, ratio: 0 },
            { value: 50, ratio: 0.5 },
            { value: 0, ratio: 1 },
        ]);

        const tooltip = timelineTooltip([
            { label: 'Allocatable headroom', data: [{ timestamp: 2_000, value: 10 }] },
            { label: 'Missing source', data: [{ timestamp: 3_000, value: 20 }] },
            { label: 'MemoryPressure=True', condition: true, data: [{ timestamp: 2_000, value: 1 }] },
        ], 2_000, 500, [{ timestamp: 2_100, reason: 'NodeNotReady', severity: 'warning' }]);

        expect(tooltip.values).toEqual([
            { label: 'Allocatable headroom', value: 10, condition: undefined },
            { label: 'MemoryPressure=True', value: 1, condition: true },
        ]);
        expect(tooltip.markers).toEqual([{ reason: 'NodeNotReady', severity: 'warning' }]);
    });
});
