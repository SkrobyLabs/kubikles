import { describe, expect, it, vi } from 'vitest';
import { createValueStore } from './valueStore';

describe('value store', () => {
    it('assign updates the value without notifying; publish notifies once per subscriber', () => {
        const store = createValueStore({ n: 1 });
        const a = vi.fn();
        const b = vi.fn();
        store.subscribe(a);
        const offB = store.subscribe(b);
        store.assign({ n: 2 });
        expect(store.get()).toEqual({ n: 2 });
        expect(a).not.toHaveBeenCalled();
        store.publish();
        expect(a).toHaveBeenCalledTimes(1);
        expect(b).toHaveBeenCalledTimes(1);
        offB();
        store.publish();
        expect(a).toHaveBeenCalledTimes(2);
        expect(b).toHaveBeenCalledTimes(1);
    });
});
