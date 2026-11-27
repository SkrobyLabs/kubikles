import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import { createValueStore } from '~/utils/valueStore';
import { PodMetricCell, podMetricKey, type PodMetricsSnapshot } from './PodMetricCells';

const item = { metadata: { name: 'web', namespace: 'prod' } };
const render = (snapshot: PodMetricsSnapshot, kind: 'cpu' | 'memory' = 'cpu') =>
    renderToStaticMarkup(<PodMetricCell store={createValueStore(snapshot)} item={item} kind={kind} />);

describe('PodMetricCell', () => {
    it('keys metrics by namespace/name', () => {
        expect(podMetricKey(item)).toBe('prod/web');
    });

    it('distinguishes unavailable metrics from a pod without a sample', () => {
        expect(render({ metrics: {}, available: false })).toContain('N/A');
        expect(render({ metrics: {}, available: true })).toContain('--');
        expect(render({ metrics: {}, available: null })).toContain('--');
    });

    it('renders a usage bar once the pod has a sample', () => {
        const html = render({ metrics: { 'prod/web': { cpuPercent: 40, cpuCommittedPercent: 60, memPercent: 10 } }, available: true });
        expect(html).not.toContain('--');
        expect(html).not.toContain('N/A');
    });
});
