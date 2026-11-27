import { beforeEach, describe, expect, it, vi } from 'vitest';

// useRemoteOrLegacy picks between the legacy hook and the Go view. React is replaced by a
// minimal runner that only needs memoisation; the two inner hooks are mocks.
const mocks = vi.hoisted(() => ({
    availability: 'off' as 'off' | 'pending' | 'on',
    legacy: vi.fn(),
    remote: vi.fn(),
}));

vi.mock('react', () => ({ useMemo: (factory: () => any) => factory() }));
vi.mock('../lib/views/availability', () => ({ useViewAvailability: () => mocks.availability }));
vi.mock('./useResourceView', () => ({ useResourceView: (...a: any[]) => mocks.remote(...a) }));

import { useRemoteOrLegacy, viewCapableNamespaced, viewCapableCluster } from './useRemoteOrLegacy';

const legacyResult = { configMaps: [{ metadata: { uid: 'x' } }], loading: false, error: null, loadingProgress: null, loadState: { phase: 'complete' }, refetch: vi.fn() };
const remoteResult = {
    view: { id: 'handle' }, loading: true, error: null, loadingProgress: { loaded: 1, total: 2 },
    loadState: { phase: 'loading' }, refetch: vi.fn(),
};
const args = { context: 'ctx', namespaces: ['a'], isVisible: true };

beforeEach(() => {
    mocks.availability = 'off';
    mocks.legacy.mockReset().mockReturnValue(legacyResult);
    mocks.remote.mockReset().mockReturnValue(remoteResult);
});

describe('useRemoteOrLegacy', () => {
    it('runs the legacy hook unchanged when views are off', () => {
        const res: any = useRemoteOrLegacy('configmaps', 'configMaps', mocks.legacy, args);
        expect(mocks.legacy).toHaveBeenCalledWith(true);
        expect(mocks.remote).toHaveBeenCalledWith('configmaps', expect.objectContaining({ enabled: false }));
        expect(res.configMaps).toBe(legacyResult.configMaps);
        expect(res.view).toBeNull();
        expect(res.viewMode).toBe(false);
    });

    it('serves the view and idles the legacy hook when views are on', () => {
        mocks.availability = 'on';
        const res: any = useRemoteOrLegacy('configmaps', 'configMaps', mocks.legacy, { ...args, sort: { key: 'age', direction: 'desc' }, hide: ['system'], columns: ['name'] });
        expect(mocks.legacy).toHaveBeenCalledWith(false);
        expect(mocks.remote).toHaveBeenCalledWith('configmaps', expect.objectContaining({
            enabled: true, context: 'ctx', namespaces: ['a'], sort: { key: 'age', direction: 'desc' }, hide: ['system'], columns: ['name'],
        }));
        expect(res.viewMode).toBe(true);
        expect(res.view).toBe(remoteResult.view);
        expect(res.configMaps).toEqual([]);
        expect(res.loading).toBe(true);
        expect(res.loadState).toBe(remoteResult.loadState);
        expect(res.loadingProgress).toEqual({ loaded: 1, total: 2 });
    });

    it('keeps both paths idle while the backend has not answered, reporting loading', () => {
        mocks.availability = 'pending';
        const res: any = useRemoteOrLegacy('configmaps', 'configMaps', mocks.legacy, args);
        expect(mocks.legacy).toHaveBeenCalledWith(false);
        expect(mocks.remote).toHaveBeenCalledWith('configmaps', expect.objectContaining({ enabled: false }));
        expect(res.loading).toBe(true);
        expect(res.loadState.phase).toBe('loading');
        expect(res.configMaps).toEqual([]);
        expect(res.view).toBeNull();
    });

    it('does not open a view for a list that is not visible, and idles legacy too', () => {
        mocks.availability = 'on';
        useRemoteOrLegacy('configmaps', 'configMaps', mocks.legacy, { ...args, isVisible: false });
        expect(mocks.legacy).toHaveBeenCalledWith(false);
        expect(mocks.remote).toHaveBeenCalledWith('configmaps', expect.objectContaining({ enabled: false }));
    });
});

describe('view capable wrappers', () => {
    it('leave unmigrated types exactly as they are', () => {
        const legacy = vi.fn();
        expect(viewCapableNamespaced('cronjobs', 'cronJobs', legacy as any)).toBe(legacy);
        expect(viewCapableCluster('crds', 'crds', legacy as any)).toBe(legacy);
    });

    it('wrap migrated types and pass their arguments through', () => {
        mocks.availability = 'off';
        const hook = viewCapableNamespaced('configmaps', 'configMaps', ((ctx: any, ns: any, visible: any) => mocks.legacy(ctx, ns, visible)) as any);
        const res: any = hook('ctx', ['a'], true, { sort: { key: 'age', direction: 'asc' } });
        expect(mocks.legacy).toHaveBeenCalledWith('ctx', ['a'], true);
        expect(res.view).toBeNull();

        mocks.availability = 'on';
        const cluster = viewCapableCluster('namespaces', 'namespaces', ((ctx: any, visible: any) => mocks.legacy(ctx, visible)) as any);
        cluster('ctx', true);
        expect(mocks.remote).toHaveBeenLastCalledWith('namespaces', expect.objectContaining({ namespaces: ['*'], enabled: true }));
        expect(mocks.legacy).toHaveBeenLastCalledWith('ctx', false);
    });
});
