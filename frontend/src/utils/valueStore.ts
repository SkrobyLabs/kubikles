import { useLayoutEffect, useState, useSyncExternalStore } from 'react';

/**
 * Minimal external value store with selector subscriptions. Used to hand fast-changing
 * data (metrics ticks, CRD lists) to table cells without putting it in column definitions:
 * columns stay referentially stable, and only cells whose selected value changed re-render.
 */
export interface ValueStore<T> {
    get(): T;
    /** Assigns without notifying; pair with publish() once the render has committed. */
    assign(value: T): void;
    publish(): void;
    subscribe(listener: () => void): () => void;
}

export function createValueStore<T>(initial: T): ValueStore<T> {
    let value = initial;
    const listeners = new Set<() => void>();
    return {
        get: () => value,
        assign: (next) => { value = next; },
        publish: () => { listeners.forEach(l => l()); },
        subscribe: (listener) => { listeners.add(listener); return () => { listeners.delete(listener); }; },
    };
}

export function useValueStore<T>(initial: T): ValueStore<T> {
    const [store] = useState(() => createValueStore(initial));
    return store;
}

/**
 * Keeps `store` equal to `value`. The assignment happens during render so that sort and
 * filter code running in the same render already sees the new value; subscribers are
 * notified after commit.
 */
export function useSyncValueStore<T>(store: ValueStore<T>, value: T): void {
    store.assign(value);
    useLayoutEffect(() => { store.publish(); }, [store, value]);
}

/** Subscribes to `selector(store value)`; re-renders only when the selected value changes. */
export function useStoreValue<T, S>(store: ValueStore<T>, selector: (value: T) => S): S {
    const read = () => selector(store.get());
    return useSyncExternalStore(store.subscribe, read, read);
}
