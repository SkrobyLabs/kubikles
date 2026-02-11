import { describe, it, expect, vi } from 'vitest';

// Mock Wails modules that are transitively imported
vi.mock('wailsjs/go/main/App', () => ({
    CancelListRequest: vi.fn(),
    SubscribeResourceWatcher: vi.fn(),
    SubscribeCRDWatcher: vi.fn(),
    UnsubscribeWatcher: vi.fn(),
    ListContexts: vi.fn(),
    GetCurrentContext: vi.fn(),
    SwitchContext: vi.fn(),
    TestConnection: vi.fn(),
    ListNamespaces: vi.fn(),
    StartPortForwardsWithMode: vi.fn(),
    ListCRDs: vi.fn(),
    GetK8sInitError: vi.fn(),
    SetRequestCancellationEnabled: vi.fn(),
    SetForceHTTP1: vi.fn(),
    SetClientPoolSize: vi.fn(),
    SetDebugEnabled: vi.fn(),
    GetCurrentTheme: vi.fn(),
    GetThemes: vi.fn(),
    SetTheme: vi.fn(),
    CheckAIProvider: vi.fn(),
    StartAISession: vi.fn(),
    SendAIMessage: vi.fn(),
    CancelAIRequest: vi.fn(),
    ClearAISession: vi.fn(),
    CloseAISession: vi.fn(),
    RunIssueScan: vi.fn(),
    ListIssueRules: vi.fn(),
    ReloadIssueRules: vi.fn(),
    GetIssueRulesDir: vi.fn(),
    OpenIssueRulesDir: vi.fn(),
}));

vi.mock('wailsjs/runtime/runtime', () => ({
    EventsOn: vi.fn(() => () => {}),
}));

vi.mock('wailsjs/go/models', () => ({
    main: {},
}));

import { createNamespaceKey, createNamespacedRequestId, createClusterScopedRequestId } from './useResource';

describe('createNamespacedRequestId', () => {
    it('includes resource type and namespace key', () => {
        const id = createNamespacedRequestId('pods', ['default']);
        expect(id).toMatch(/^list-pods-default-\d+$/);
    });

    it('includes sorted namespace key for multiple namespaces', () => {
        const id = createNamespacedRequestId('services', ['ns2', 'ns1']);
        expect(id).toMatch(/^list-services-ns1,ns2-\d+$/);
    });

    it('uses "all" for null namespaces', () => {
        const id = createNamespacedRequestId('pods', null);
        expect(id).toMatch(/^list-pods-all-\d+$/);
    });

    it('generates unique IDs on successive calls', () => {
        const id1 = createNamespacedRequestId('pods', ['default']);
        const id2 = createNamespacedRequestId('pods', ['default']);
        expect(id1).not.toBe(id2);
    });

    it('generates unique IDs even for different resource types', () => {
        const id1 = createNamespacedRequestId('pods', ['default']);
        const id2 = createNamespacedRequestId('services', ['default']);
        expect(id1).not.toBe(id2);
    });
});

describe('createClusterScopedRequestId', () => {
    it('includes resource type and cluster marker', () => {
        const id = createClusterScopedRequestId('nodes');
        expect(id).toMatch(/^list-nodes-cluster-\d+$/);
    });

    it('generates unique IDs on successive calls', () => {
        const id1 = createClusterScopedRequestId('nodes');
        const id2 = createClusterScopedRequestId('nodes');
        expect(id1).not.toBe(id2);
    });
});

describe('createNamespaceKey', () => {
    it('returns sorted comma-joined key for array', () => {
        expect(createNamespaceKey(['ns2', 'ns1'])).toBe('ns1,ns2');
    });

    it('returns "all" for empty array', () => {
        expect(createNamespaceKey([])).toBe('all');
    });

    it('returns string as-is for non-array', () => {
        expect(createNamespaceKey('default')).toBe('default');
    });

    it('returns "all" for null', () => {
        expect(createNamespaceKey(null)).toBe('all');
    });

    it('returns "all" for undefined', () => {
        expect(createNamespaceKey(undefined)).toBe('all');
    });
});

describe('ghost reconciliation filtering logic', () => {
    // These tests exercise the exact filtering logic used inside useGhostReconciliation's
    // setData updater: given local state and a set of fresh UIDs from the cluster,
    // only items whose UID is NOT in the fresh set are removed (ghosts).

    const makeResource = (uid, name = 'resource') => ({
        metadata: { uid, name }
    });

    /**
     * Simulates the ghost reconciliation filter from useGhostReconciliation.
     * This is the exact logic extracted from the setData updater callback.
     */
    function ghostFilter(prev, freshUids) {
        const filtered = prev.filter(r => {
            const uid = r.metadata?.uid;
            if (!uid) return true;
            return freshUids.has(uid);
        });
        const removed = prev.length - filtered.length;
        return removed > 0 ? filtered : prev;
    }

    it('removes ghost resources not present in fresh set', () => {
        const local = [
            makeResource('uid-1', 'pod-1'),
            makeResource('uid-2', 'pod-2'),  // ghost
            makeResource('uid-3', 'pod-3'),
        ];
        const freshUids = new Set(['uid-1', 'uid-3']);

        const result = ghostFilter(local, freshUids);
        expect(result).toEqual([
            makeResource('uid-1', 'pod-1'),
            makeResource('uid-3', 'pod-3'),
        ]);
    });

    it('returns same reference when no ghosts found', () => {
        const local = [
            makeResource('uid-1', 'pod-1'),
            makeResource('uid-2', 'pod-2'),
        ];
        const freshUids = new Set(['uid-1', 'uid-2']);

        const result = ghostFilter(local, freshUids);
        expect(result).toBe(local); // Same reference — no re-render
    });

    it('preserves items without UID', () => {
        const noUid = { metadata: { name: 'no-uid-resource' } };
        const local = [
            makeResource('uid-1', 'pod-1'),
            noUid,
        ];
        const freshUids = new Set(['uid-1']);

        const result = ghostFilter(local, freshUids);
        expect(result).toBe(local); // No change — noUid item kept
    });

    it('removes all items when fresh set is empty (everything is a ghost)', () => {
        const local = [
            makeResource('uid-1', 'pod-1'),
            makeResource('uid-2', 'pod-2'),
        ];
        const freshUids = new Set();

        const result = ghostFilter(local, freshUids);
        expect(result).toEqual([]);
    });

    it('handles empty local state', () => {
        const local = [];
        const freshUids = new Set(['uid-1', 'uid-2']);

        const result = ghostFilter(local, freshUids);
        expect(result).toBe(local); // Same reference
    });

    it('handles fresh set with extra UIDs not in local state', () => {
        const local = [
            makeResource('uid-1', 'pod-1'),
        ];
        const freshUids = new Set(['uid-1', 'uid-2', 'uid-3']);

        const result = ghostFilter(local, freshUids);
        expect(result).toBe(local); // Same reference
    });

    it('removes multiple ghosts in one pass', () => {
        const local = [
            makeResource('uid-1', 'pod-1'),  // ghost
            makeResource('uid-2', 'pod-2'),  // alive
            makeResource('uid-3', 'pod-3'),  // ghost
            makeResource('uid-4', 'pod-4'),  // ghost
            makeResource('uid-5', 'pod-5'),  // alive
        ];
        const freshUids = new Set(['uid-2', 'uid-5']);

        const result = ghostFilter(local, freshUids);
        expect(result).toEqual([
            makeResource('uid-2', 'pod-2'),
            makeResource('uid-5', 'pod-5'),
        ]);
    });

    it('preserves order of remaining items', () => {
        const local = [
            makeResource('uid-3', 'pod-3'),
            makeResource('uid-1', 'pod-1'),  // ghost
            makeResource('uid-2', 'pod-2'),
        ];
        const freshUids = new Set(['uid-3', 'uid-2']);

        const result = ghostFilter(local, freshUids);
        expect(result).toEqual([
            makeResource('uid-3', 'pod-3'),
            makeResource('uid-2', 'pod-2'),
        ]);
    });
});
