import { describe, it, expect } from 'vitest';
import { startBackendVisibilitySync } from './backendVisibility';

function harness(initial = true) {
    let visible = initial;
    let cb: ((v: boolean) => void) | null = null;
    const sent: boolean[] = [];
    const timers: { fn: () => void; id: number; live: boolean }[] = [];
    const stop = startBackendVisibilitySync({
        send: (v) => { sent.push(v); },
        getSnapshot: () => visible,
        subscribe: (c) => { cb = c; return () => { cb = null; }; },
        setTimer: (fn) => { const t = { fn, id: timers.length, live: true }; timers.push(t); return t; },
        clearTimer: (h) => { (h as any).live = false; },
    });
    return {
        stop, sent, timers,
        set(v: boolean) { visible = v; cb?.(v); },
        fire() { timers.filter((t) => t.live).forEach((t) => { t.live = false; t.fn(); }); },
        subscribed: () => cb !== null,
    };
}

describe('backend visibility sync', () => {
    it('debounces hide and sends nothing for a quick hide/show', () => {
        const h = harness();
        h.set(false);
        h.set(true);
        h.fire();
        expect(h.sent).toEqual([]);
    });
    it('sends hide after the debounce and show immediately', () => {
        const h = harness();
        h.set(false);
        expect(h.sent).toEqual([]);
        h.fire();
        expect(h.sent).toEqual([false]);
        h.set(true);
        expect(h.sent).toEqual([false, true]);
    });
    it('reports initially hidden state', () => {
        const h = harness(false);
        h.fire();
        expect(h.sent).toEqual([false]);
    });
    it('unsubscribes and cancels pending hide on stop', () => {
        const h = harness();
        h.set(false);
        h.stop();
        h.fire();
        expect(h.sent).toEqual([]);
        expect(h.subscribed()).toBe(false);
    });
});
