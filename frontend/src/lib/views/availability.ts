import { useEffect, useSyncExternalStore } from 'react';
import { useConfig } from '~/context/ConfigContext';
import { hasRowSchema, isDynamicType } from './schema';
import { loadViewsInfo } from './transport';
import type { ViewsInfo } from './protocol';

// Whether a resource type is served by a Go view in this session. Three inputs decide it: the
// viewStore setting, what the backend reports (enabled, registered types), and a session-level
// list of kinds whose view repeatedly failed (those fall back to the legacy hook).

let infoPromise: Promise<ViewsInfo> | null = null;
let info: ViewsInfo | null = null;
let infoVersion = 0;
const listeners = new Set<() => void>();
const degraded = new Set<string>();

function notify(): void {
    infoVersion++;
    for (const l of [...listeners]) l();
}

/** Loads the backend's view info once per session. A failure counts as "views unavailable". */
export function ensureViewsInfo(): Promise<ViewsInfo> {
    if (!infoPromise) {
        infoPromise = loadViewsInfo()
            .catch(() => ({ enabled: false, types: [] as string[] }))
            .then(i => { info = i; notify(); return i; });
    }
    return infoPromise;
}

export function resetViewsInfo(): void {
    infoPromise = null;
    info = null;
    degraded.clear();
    notify();
}

export function markKindDegraded(resourceType: string): void {
    if (degraded.has(resourceType)) return;
    degraded.add(resourceType);
    notify();
}

export function isKindDegraded(resourceType: string): boolean {
    return degraded.has(resourceType);
}

function subscribe(cb: () => void): () => void {
    listeners.add(cb);
    return () => { listeners.delete(cb); };
}

function getVersion(): number {
    return infoVersion;
}

export type ViewAvailability = 'off' | 'pending' | 'on';

/** Pure decision, kept separate for tests. */
export function decideAvailability(
    resourceType: string, setting: boolean, backend: ViewsInfo | null, degradedKinds: ReadonlySet<string>,
): ViewAvailability {
    if (!setting) return 'off';
    const dynamic = isDynamicType(resourceType);
    if (!dynamic && !hasRowSchema(resourceType)) return 'off';
    if (degradedKinds.has(resourceType)) return 'off';
    if (!backend) return 'pending';
    if (!backend.enabled) return 'off';
    if (dynamic) return backend.dynamic ? 'on' : 'off';
    if (!backend.types.includes(resourceType)) return 'off';
    return 'on';
}

/**
 * 'pending' while the backend has not answered yet: callers keep both paths idle for that
 * moment so a legacy list never starts and then switches over.
 */
export function useViewAvailability(resourceType: string): ViewAvailability {
    const { getConfig } = useConfig();
    const setting = getConfig('performance.viewStore') === true;
    useSyncExternalStore(subscribe, getVersion, getVersion);
    useEffect(() => {
        if (setting && (hasRowSchema(resourceType) || isDynamicType(resourceType))) void ensureViewsInfo();
    }, [setting, resourceType]);
    return decideAvailability(resourceType, setting, info, degraded);
}
