import { useMemo, useRef } from 'react';

/**
 * Returns the given object with every function replaced by a wrapper whose identity never
 * changes but which always calls the latest function. Lets column definitions (which must
 * stay referentially stable so the table does not re-filter and re-sort) use action hooks
 * that recreate their handlers on every render. Non-function values pass through as is.
 */
export function useStableCallbacks<T extends Record<string, any>>(source: T): T {
    const ref = useRef(source);
    ref.current = source;
    const functionKeys = Object.keys(source).filter(k => typeof source[k] === 'function');
    const shape = functionKeys.join('|');
    const stable = useMemo(() => {
        const wrappers: Record<string, (...args: any[]) => any> = {};
        for (const key of shape ? shape.split('|') : []) {
            wrappers[key] = (...args: any[]) => ref.current[key]?.(...args);
        }
        return wrappers;
    }, [shape]);
    const result: Record<string, any> = { ...source };
    for (const key of functionKeys) result[key] = stable[key];
    return result as T;
}
