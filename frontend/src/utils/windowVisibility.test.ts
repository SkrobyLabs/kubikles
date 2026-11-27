import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';

type Handler = () => void;

function installDom() {
    const handlers: Record<string, Set<Handler>> = {};
    const add = (t: string, h: Handler) => { (handlers[t] ||= new Set()).add(h); };
    const remove = (t: string, h: Handler) => { handlers[t]?.delete(h); };
    const doc: any = { visibilityState: 'visible', addEventListener: add, removeEventListener: remove };
    const win: any = { addEventListener: add, removeEventListener: remove };
    (globalThis as any).document = doc;
    (globalThis as any).window = win;
    return {
        doc,
        handlers,
        setVisible(v: boolean) {
            doc.visibilityState = v ? 'visible' : 'hidden';
            handlers['visibilitychange']?.forEach(h => h());
        },
    };
}

describe('windowVisibility', () => {
    let dom: ReturnType<typeof installDom>;
    beforeEach(() => {
        vi.resetModules();
        dom = installDom();
    });
    afterEach(() => {
        delete (globalThis as any).document;
        delete (globalThis as any).window;
    });

    it('notifies subscribers only on change and detaches listeners when unused', async () => {
        const m = await import('./windowVisibility');
        const cb = vi.fn();
        const off = m.subscribeWindowVisibility(cb);
        dom.setVisible(true);
        expect(cb).not.toHaveBeenCalled();
        dom.setVisible(false);
        expect(cb).toHaveBeenLastCalledWith(false);
        expect(m.getWindowVisibleSnapshot()).toBe(false);
        dom.setVisible(true);
        expect(cb).toHaveBeenLastCalledWith(true);
        off();
        expect(dom.handlers['visibilitychange'].size).toBe(0);
        expect(dom.handlers['focus'].size).toBe(0);
    });

    it('gate skips ticks while hidden and catches up once on show', async () => {
        const m = await import('./windowVisibility');
        const run = vi.fn();
        const gate = m.createVisibilityGate(run);
        gate.tick();
        expect(run).toHaveBeenCalledTimes(1);
        dom.setVisible(false);
        gate.tick();
        gate.tick();
        expect(run).toHaveBeenCalledTimes(1);
        dom.setVisible(true);
        expect(run).toHaveBeenCalledTimes(2);
        dom.setVisible(false);
        dom.setVisible(true);
        expect(run).toHaveBeenCalledTimes(2);
        gate.dispose();
    });

    it('defaults to visible without a document', async () => {
        delete (globalThis as any).document;
        const m = await import('./windowVisibility');
        expect(m.getWindowVisibleSnapshot()).toBe(true);
        const off = m.subscribeWindowVisibility(() => {});
        off();
    });

    it('startVisibleInterval runs immediately, ticks, pauses while hidden and stops', async () => {
        vi.useFakeTimers();
        try {
            const m = await import('./windowVisibility');
            const run = vi.fn();
            const stop = m.startVisibleInterval(run, 1000);
            expect(run).toHaveBeenCalledTimes(1);
            vi.advanceTimersByTime(2000);
            expect(run).toHaveBeenCalledTimes(3);
            dom.setVisible(false);
            vi.advanceTimersByTime(5000);
            expect(run).toHaveBeenCalledTimes(3);
            dom.setVisible(true);
            expect(run).toHaveBeenCalledTimes(4);
            stop();
            vi.advanceTimersByTime(5000);
            expect(run).toHaveBeenCalledTimes(4);
        } finally {
            vi.useRealTimers();
        }
    });
});
