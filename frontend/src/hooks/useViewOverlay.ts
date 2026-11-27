import { useEffect } from 'react';
import { SetMetricsOverlayConfig } from 'wailsjs/go/main/App';
import { useConfig } from '../context';
import type { ViewSnapshot } from '../lib/views/viewClient';
import { useValueStore, useSyncValueStore, type ValueStore } from '../utils/valueStore';

/**
 * Tells the backend which metrics source and poll interval the user configured, so the metrics
 * overlays of a view follow the same settings as the legacy metrics hooks. The backend polls
 * only while a view wants the source and the window is visible; nothing is sent while the list
 * is not in view mode.
 */
export function useMetricsOverlayConfig(enabled: boolean): void {
    const { getConfig } = useConfig();
    const preferred = (getConfig('metrics.preferredSource') ?? 'auto') as string;
    const interval = (getConfig('kubernetes.metricsPollIntervalMs') ?? 30000) as number;
    useEffect(() => {
        if (!enabled) return;
        try {
            Promise.resolve(SetMetricsOverlayConfig(preferred, interval)).catch(() => { /* the backend keeps its defaults */ });
        } catch { /* binding missing in this build */ }
    }, [enabled, preferred, interval]);
}

/**
 * The "metricsAvailable" state the legacy lists show (N/A cells) for one overlay source of a
 * view, as a store the cells subscribe to: a change re-renders the metric cells only and leaves
 * the column definitions untouched. null until the source answered for the first time.
 */
export function useOverlayAvailability(snapshot: ViewSnapshot | null | undefined, source: string): ValueStore<boolean | null> {
    const value = snapshot?.state?.overlays?.[source]?.available ?? null;
    const store = useValueStore<boolean | null>(value);
    useSyncValueStore(store, value);
    return store;
}
