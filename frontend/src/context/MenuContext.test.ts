import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createMenuStore } from './MenuContext';

const button = (rect: Partial<DOMRect>) => ({ getBoundingClientRect: () => ({ left: 0, right: 0, top: 0, bottom: 0, ...rect }) }) as unknown as HTMLElement;

describe('menu store', () => {
    beforeEach(() => { vi.stubGlobal('window', { innerWidth: 1000, innerHeight: 800 }); });
    afterEach(() => { vi.unstubAllGlobals(); });

    it('notifies subscribers only when the open menu or placement changes', () => {
        const store = createMenuStore();
        const listener = vi.fn();
        store.subscribe(listener);
        store.setActiveMenuId(null);
        expect(listener).not.toHaveBeenCalled();
        store.setActiveMenuId('pod-1');
        expect(listener).toHaveBeenCalledTimes(1);
        store.setActiveMenuId(prev => prev);
        expect(listener).toHaveBeenCalledTimes(1);
        store.changeMenu(false, 'pod-1', null);
        expect(store.getState().activeMenuId).toBeNull();
        expect(listener).toHaveBeenCalledTimes(2);
    });

    it('unsubscribes', () => {
        const store = createMenuStore();
        const listener = vi.fn();
        const off = store.subscribe(listener);
        off();
        store.setActiveMenuId('x');
        expect(listener).not.toHaveBeenCalled();
    });

    it('opens below the button, right-aligned, when there is room', () => {
        const store = createMenuStore();
        store.changeMenu(true, 'pod-1', button({ left: 700, right: 740, top: 100, bottom: 130 }));
        const { activeMenuId, menuPosition } = store.getState();
        expect(activeMenuId).toBe('pod-1');
        expect(menuPosition.left).toBe(740 - 192);
        expect(menuPosition.top).toBe(134);
        expect(menuPosition.bottom).toBeUndefined();
    });

    it('opens upwards near the bottom edge and clamps to the left margin', () => {
        const store = createMenuStore();
        store.changeMenu(true, 'pod-1', button({ left: 0, right: 20, top: 700, bottom: 730 }));
        const { menuPosition } = store.getState();
        expect(menuPosition.left).toBe(8);
        expect(menuPosition.bottom).toBe(800 - 700 + 4);
        expect(menuPosition.top).toBeUndefined();
    });

    it('keeps the previous placement when closing', () => {
        const store = createMenuStore();
        store.changeMenu(true, 'a', button({ left: 700, right: 740, top: 100, bottom: 130 }));
        const placed = store.getState().menuPosition;
        store.changeMenu(false, 'a', null);
        expect(store.getState().menuPosition).toBe(placed);
    });
});
