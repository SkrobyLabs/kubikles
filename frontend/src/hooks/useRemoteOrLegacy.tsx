import { useMemo } from 'react';
import { useViewAvailability } from '../lib/views/availability';
import { hasRowSchema } from '../lib/views/schema';
import type { ViewHandle, ViewSnapshot } from '../lib/views/viewClient';
import type { CrdDescriptor } from '../lib/views/protocol';
import { useResourceView } from './useResourceView';
import type { LoadingProgress, ResourceLoadState } from './useResource';

/** Query seed for a view: the list component owns the live values afterwards. */
export interface ViewOptions {
    sort?: { key: string | null; direction: string };
    /** Row presets to hide ("helm" on secrets, "system" on configmaps). */
    hide?: string[];
    /** Schema columns shown by default; seeds the projection so no patch follows the open. */
    columns?: string[];
    /** Custom resource descriptor for "crd:" types. */
    crd?: CrdDescriptor;
    /** Event kinds the view reports in notices ("pods" on the pod list: restarts, vanished and new pods). */
    notifyOn?: string[];
}

export interface RemoteOrLegacyArgs extends ViewOptions {
    context: string | null;
    namespaces: string | string[] | null | undefined;
    isVisible: boolean;
}

interface CommonState {
    loading: boolean;
    error: Error | null;
    loadingProgress: LoadingProgress | null;
    loadState: ResourceLoadState;
    refetch: () => void | Promise<void>;
}

const EMPTY: never[] = [];
const noop = () => {};

/**
 * One list hook with two data sources. When the viewStore setting is on, the backend serves the
 * type and nothing failed, the Go view feeds the list (`view` is the handle, the state array is
 * empty). Otherwise the legacy hook runs unchanged and `view` is null. Both hooks are always
 * called; the inactive one is kept idle through its visibility flag.
 */
export function useRemoteOrLegacy<R extends object>(
    resourceType: string,
    stateName: string,
    useLegacy: (visible: boolean) => R,
    args: RemoteOrLegacyArgs,
): R & { view: ViewHandle | null; viewMode: boolean; viewSnapshot: ViewSnapshot | null } {
    const availability = useViewAvailability(resourceType);
    const legacy = useLegacy(args.isVisible && availability === 'off');
    const remote = useResourceView(resourceType, {
        enabled: availability === 'on' && args.isVisible,
        context: args.context,
        namespaces: args.namespaces,
        sort: args.sort,
        hide: args.hide,
        columns: args.columns,
        crd: args.crd,
        notifyOn: args.notifyOn,
    });

    const pendingState = useMemo<ResourceLoadState>(
        () => ({ phase: 'loading', error: null, progress: null, retry: noop }), []);

    return useMemo(() => {
        if (availability === 'off') return { ...legacy, view: null, viewMode: false, viewSnapshot: null };
        if (availability === 'pending') {
            return {
                ...legacy, [stateName]: EMPTY, loading: true, error: null, loadingProgress: null,
                loadState: pendingState, view: null, viewMode: false, viewSnapshot: null,
            } as R & { view: ViewHandle | null; viewMode: boolean; viewSnapshot: ViewSnapshot | null };
        }
        const common: CommonState = {
            loading: remote.loading, error: remote.error, loadingProgress: remote.loadingProgress,
            loadState: remote.loadState, refetch: remote.refetch,
        };
        return { ...legacy, ...common, [stateName]: EMPTY, view: remote.view, viewMode: true, viewSnapshot: remote.snapshot } as R & { view: ViewHandle | null; viewMode: boolean; viewSnapshot: ViewSnapshot | null };
    }, [availability, legacy, remote, stateName, pendingState]);
}

type NamespacedLegacy<R> = (ctx: string | null, namespaces: string | string[] | null | undefined, isVisible: boolean) => R;
type ClusterLegacy<R> = (ctx: string | null, isVisible: boolean) => R;

/** Wraps a namespaced legacy hook; unmigrated types (no generated schema) are returned as is. */
export function viewCapableNamespaced<R extends object>(resourceType: string, stateName: string, legacy: NamespacedLegacy<R>) {
    if (!hasRowSchema(resourceType)) return legacy as (ctx: string | null, ns: string | string[] | null | undefined, isVisible: boolean, view?: ViewOptions) => R & { view?: ViewHandle | null; viewMode?: boolean };
    return function useViewCapable(ctx: string | null, ns: string | string[] | null | undefined, isVisible: boolean, view?: ViewOptions) {
        return useRemoteOrLegacy(resourceType, stateName, visible => legacy(ctx, ns, visible),
            { context: ctx, namespaces: ns, isVisible, ...view });
    };
}

export function viewCapableCluster<R extends object>(resourceType: string, stateName: string, legacy: ClusterLegacy<R>) {
    if (!hasRowSchema(resourceType)) return legacy as (ctx: string | null, isVisible: boolean, view?: ViewOptions) => R & { view?: ViewHandle | null; viewMode?: boolean };
    return function useViewCapable(ctx: string | null, isVisible: boolean, view?: ViewOptions) {
        return useRemoteOrLegacy(resourceType, stateName, visible => legacy(ctx, visible),
            { context: ctx, namespaces: ['*'], isVisible, ...view });
    };
}
