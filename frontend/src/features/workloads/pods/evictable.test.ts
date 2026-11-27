import { describe, it, expect } from 'vitest';
import { isEvictablePod, evictablePods } from './evictable';

describe('evictable pods', () => {
    it('excludes succeeded, failed and terminating pods', () => {
        const running = { status: { phase: 'Running' } };
        const pending = { status: { phase: 'Pending' } };
        const done = { status: { phase: 'Succeeded' } };
        const failed = { status: { phase: 'Failed' } };
        const dying = { metadata: { deletionTimestamp: '2025-01-01T00:00:00Z' }, status: { phase: 'Running' } };
        expect(evictablePods([running, pending, done, failed, dying])).toEqual([running, pending]);
        expect(isEvictablePod(dying)).toBe(false);
    });
    it('is empty when nothing qualifies, so no modal opens', () => {
        expect(evictablePods([{ status: { phase: 'Failed' } }])).toEqual([]);
    });
});
