import { describe, it, expect, vi, beforeEach } from 'vitest';

/**
 * Tests for the event subscription lifecycle used by useResourceWatcher.
 *
 * The core issue: Wails v2 EventsOff(eventName) removes ALL listeners for that
 * event name, not just a specific callback. When useResourceWatcher's cleanup
 * calls EventsOff("resource-event"), it kills every other watcher's listener too.
 *
 * The fix: use the cancel function returned by EventsOn() for targeted removal.
 *
 * These tests use a minimal event bus that faithfully reproduces the Wails v2
 * EventsOn/EventsOff semantics (matches both desktop and server mode behavior).
 */

// Minimal event bus matching Wails v2 EventsOn/EventsOff semantics
function createEventBus() {
    const listeners = new Map(); // eventName -> Set<callback>

    function eventsOn(eventName, callback) {
        if (!listeners.has(eventName)) listeners.set(eventName, new Set());
        listeners.get(eventName).add(callback);
        // EventsOn returns a cancel function for targeted removal
        return () => listeners.get(eventName)?.delete(callback);
    }

    // Wails v2 EventsOff: removes ALL listeners for the event name
    function eventsOff(eventName, ...additionalEventNames) {
        [eventName, ...additionalEventNames].forEach(name => listeners.delete(name));
    }

    function dispatch(eventName, data) {
        const set = listeners.get(eventName);
        if (set) set.forEach(cb => cb(data));
    }

    function clear() {
        listeners.clear();
    }

    return { eventsOn, eventsOff, dispatch, clear };
}

describe('Event subscription lifecycle', () => {
    let bus;
    beforeEach(() => {
        bus = createEventBus();
    });

    describe('EventsOn cancel function', () => {
        it('removes only the specific listener', () => {
            const handler1 = vi.fn();
            const handler2 = vi.fn();

            const cancel1 = bus.eventsOn("test-event", handler1);
            bus.eventsOn("test-event", handler2);

            cancel1();

            bus.dispatch("test-event", { value: 1 });

            expect(handler1).not.toHaveBeenCalled();
            expect(handler2).toHaveBeenCalledWith({ value: 1 });
        });

        it('is idempotent', () => {
            const handler = vi.fn();
            const cancel = bus.eventsOn("test-event", handler);

            cancel();
            cancel(); // Should not throw

            bus.dispatch("test-event", {});
            expect(handler).not.toHaveBeenCalled();
        });
    });

    describe('EventsOff removes all listeners (documents Wails v2 behavior)', () => {
        it('removes every listener for the given event name', () => {
            const handler1 = vi.fn();
            const handler2 = vi.fn();

            bus.eventsOn("test-event", handler1);
            bus.eventsOn("test-event", handler2);

            bus.eventsOff("test-event");

            bus.dispatch("test-event", { value: 1 });

            expect(handler1).not.toHaveBeenCalled();
            expect(handler2).not.toHaveBeenCalled();
        });
    });
});

describe('useResourceWatcher cleanup pattern', () => {
    let bus;
    beforeEach(() => {
        bus = createEventBus();
    });

    /**
     * Simulates what useResourceWatcher does:
     *   mount:   EventsOn("resource-event", handleEvent)
     *   unmount: cleanup removes the listener
     *
     * Two watchers (e.g. pods + deployments) both listen to "resource-event"
     * and filter by resourceType in their handler. Unmounting one must not
     * break the other.
     */

    // Helper: simulates useResourceWatcher mount (returns cleanup function)
    function mountWatcher(resourceType, handler) {
        const handleEvent = (event) => {
            if (event.resourceType === resourceType) handler(event);
        };
        const handleBatch = (events) => {
            for (const event of events) {
                if (event.resourceType === resourceType) handler(event);
            }
        };

        const cancelEvent = bus.eventsOn("resource-event", handleEvent);
        const cancelBatch = bus.eventsOn("resource-events-batch", handleBatch);

        // Return cleanup that mimics current useResourceWatcher (uses EventsOff)
        return {
            cleanupFixed: () => {
                bus.eventsOff("resource-event", handleEvent);
                bus.eventsOff("resource-events-batch", handleBatch);
            },
            cleanupFixed: () => {
                cancelEvent();
                cancelBatch();
            },
        };
    }

    it('unmounting one watcher must not break other watchers (single events)', () => {
        const podsHandler = vi.fn();
        const deploymentsHandler = vi.fn();

        const podsWatcher = mountWatcher('pods', podsHandler);
        mountWatcher('deployments', deploymentsHandler);

        // Both receive their events before cleanup
        bus.dispatch("resource-event", { resourceType: 'pods', type: 'MODIFIED' });
        bus.dispatch("resource-event", { resourceType: 'deployments', type: 'MODIFIED' });
        expect(podsHandler).toHaveBeenCalledTimes(1);
        expect(deploymentsHandler).toHaveBeenCalledTimes(1);
        podsHandler.mockClear();
        deploymentsHandler.mockClear();

        // Pods watcher unmounts (current buggy cleanup)
        podsWatcher.cleanupFixed();

        // Deployments watcher must still receive events
        bus.dispatch("resource-event", { resourceType: 'deployments', type: 'ADDED' });
        expect(deploymentsHandler).toHaveBeenCalledTimes(1);
    });

    it('unmounting one watcher must not break other watchers (batch events)', () => {
        const podsHandler = vi.fn();
        const deploymentsHandler = vi.fn();

        const podsWatcher = mountWatcher('pods', podsHandler);
        mountWatcher('deployments', deploymentsHandler);

        // Pods watcher unmounts
        podsWatcher.cleanupFixed();

        // Deployments watcher must still receive batch events
        bus.dispatch("resource-events-batch", [
            { resourceType: 'deployments', type: 'MODIFIED' },
        ]);
        expect(deploymentsHandler).toHaveBeenCalledTimes(1);
    });

    it('closing a detail panel must not break the list view watcher', () => {
        const listHandler = vi.fn();
        const detailHandler = vi.fn();

        mountWatcher('deployments', listHandler);
        const detailWatcher = mountWatcher('deployments', detailHandler);

        // User closes detail panel
        detailWatcher.cleanupFixed();

        // List view watcher must still receive events
        bus.dispatch("resource-event", { resourceType: 'deployments', type: 'MODIFIED' });
        expect(listHandler).toHaveBeenCalledTimes(1);
    });

    it('DeploymentList pods watcher cleanup must not break deployments watcher', () => {
        // DeploymentList subscribes to BOTH deployments and pods watchers
        const deploymentsHandler = vi.fn();
        const podsHandler = vi.fn();

        mountWatcher('deployments', deploymentsHandler);
        const podsWatcher = mountWatcher('pods', podsHandler);

        // Navigate away — pods cleanup runs first
        podsWatcher.cleanupFixed();

        // Deployments watcher must still work between the two cleanups
        bus.dispatch("resource-event", { resourceType: 'deployments', type: 'MODIFIED' });
        expect(deploymentsHandler).toHaveBeenCalledTimes(1);
    });
});
