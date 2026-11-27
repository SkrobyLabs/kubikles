import { describe, expect, it, vi } from 'vitest';
import { forgetStream, rememberStream, stopLeftoverStreams } from './streamLedger';

function memoryStore() {
    const data = new Map<string, string>();
    return {
        getItem: (k: string) => data.get(k) ?? null,
        setItem: (k: string, v: string) => { data.set(k, v); },
        removeItem: (k: string) => { data.delete(k); },
        data,
    };
}

describe('streamLedger', () => {
    it('stops the streams a previous page left behind, once', () => {
        const store = memoryStore();
        rememberStream('ns/pod/a-1', store);
        rememberStream('ns/pod/b-2', store);
        rememberStream('ns/pod/a-1', store);
        // The page reloads: the store is all that is left of it.
        const stop = vi.fn();
        expect(stopLeftoverStreams(stop, store)).toBe(2);
        expect(stop.mock.calls.map(c => c[0])).toEqual(['ns/pod/a-1', 'ns/pod/b-2']);
        expect(stopLeftoverStreams(stop, store)).toBe(0);
        expect(stop).toHaveBeenCalledTimes(2);
    });

    it('does not stop streams that were stopped normally', () => {
        const store = memoryStore();
        rememberStream('a', store);
        rememberStream('b', store);
        forgetStream('a', store);
        const stop = vi.fn();
        stopLeftoverStreams(stop, store);
        expect(stop.mock.calls.map(c => c[0])).toEqual(['b']);
    });

    it('survives unreadable storage and a failing stop', () => {
        const store = memoryStore();
        store.data.set('kubikles_log_streams', '{not json');
        expect(stopLeftoverStreams(vi.fn(), store)).toBe(0);
        rememberStream('a', store);
        rememberStream('b', store);
        const stop = vi.fn(() => { throw new Error('gone'); });
        expect(stopLeftoverStreams(stop, store)).toBe(2);
    });

    it('does nothing without a store (server mode)', () => {
        const stop = vi.fn();
        rememberStream('a', null);
        expect(stopLeftoverStreams(stop, null)).toBe(0);
        expect(stop).not.toHaveBeenCalled();
    });
});
