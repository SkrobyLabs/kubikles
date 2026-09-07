import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { describe, expect, it } from 'vitest';
import ResourceLoadingStatus from './ResourceLoadingStatus';
import type { ResourceLoadState } from '~/hooks/useResource';
const render = (overrides: Partial<ResourceLoadState>) => renderToStaticMarkup(<ResourceLoadingStatus displayed={12} state={{ phase: 'loading', error: null, retry: () => {}, progress: null, ...overrides }} />);
describe('resource loading feedback', () => {
    it('keeps the incomplete-list notice even when the last page count is reached', () => {
        const html = render({ progress: { loaded: 100, total: 100 } });
        expect(html).toContain('Loading resources');
        expect(html).toContain('100 fetched of 100');
        expect(html).toContain('More resources may arrive');
        expect(html).toContain('12 displayed');
        expect(html).toContain('select-all apply to displayed resources');
    });
    it('does not invent a total for an unknown-size list', () => {
        const html = render({ progress: { loaded: 100, total: 0 } });
        expect(html).toContain('100 fetched');
        expect(html).not.toContain('fetched of');
    });
    it('distinguishes refresh, failure and successful completion', () => {
        expect(render({ phase: 'refreshing' })).toContain('Refreshing resources');
        expect(render({ phase: 'incomplete', error: new Error('offline') })).toContain('Retry');
        expect(render({ phase: 'incomplete' })).toContain('incomplete or outdated');
        expect(render({ phase: 'complete' })).toBe('');
    });
});
