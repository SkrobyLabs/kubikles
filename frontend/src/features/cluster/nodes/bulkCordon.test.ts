import { describe, it, expect } from 'vitest';
import { hasCordonTargets } from './bulkCordon';

const node = (unschedulable?: boolean) => ({ spec: unschedulable === undefined ? {} : { unschedulable } });

describe('hasCordonTargets', () => {
    it('cordon needs a schedulable node, uncordon a cordoned one', () => {
        const mixed = [node(true), node(), node(false)];
        expect(hasCordonTargets(mixed, 'cordon')).toBe(true);
        expect(hasCordonTargets(mixed, 'uncordon')).toBe(true);
    });
    it('is false when every selected node is already in the target state', () => {
        expect(hasCordonTargets([node(true), node(true)], 'cordon')).toBe(false);
        expect(hasCordonTargets([node(), node(false)], 'uncordon')).toBe(false);
        expect(hasCordonTargets([], 'cordon')).toBe(false);
    });
});
