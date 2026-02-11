import { describe, it, expect, vi } from 'vitest';
import { createResourceEventHandler, createNamespacedResourceEventHandler } from './useResourceEventHandler';

describe('createResourceEventHandler', () => {
    const createMockResource = (uid, name = 'test-resource') => ({
        metadata: { uid, name }
    });

    describe('ADDED events', () => {
        it('adds new resource to empty list', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1', 'resource-1');

            handler({ type: 'ADDED', resource });

            // Get the updater function and call it with empty prev state
            const updater = setState.mock.calls[0][0];
            const result = updater([]);
            expect(result).toEqual([resource]);
        });

        it('adds new resource to existing list', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const existing = createMockResource('uid-1', 'resource-1');
            const newResource = createMockResource('uid-2', 'resource-2');

            handler({ type: 'ADDED', resource: newResource });

            const updater = setState.mock.calls[0][0];
            const result = updater([existing]);
            expect(result).toEqual([existing, newResource]);
        });

        it('does not add duplicate resource (same uid)', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1', 'resource-1');

            handler({ type: 'ADDED', resource });

            const updater = setState.mock.calls[0][0];
            const result = updater([resource]);
            expect(result).toEqual([resource]); // Same reference, no change
        });

        it('ignores resource without uid', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = { metadata: { name: 'no-uid' } };

            handler({ type: 'ADDED', resource });

            const updater = setState.mock.calls[0][0];
            const prev = [createMockResource('uid-1')];
            const result = updater(prev);
            expect(result).toBe(prev); // Same reference, no change
        });
    });

    describe('MODIFIED events', () => {
        it('updates existing resource', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const original = createMockResource('uid-1', 'original');
            const updated = { ...createMockResource('uid-1', 'updated'), spec: { replicas: 3 } };

            handler({ type: 'MODIFIED', resource: updated });

            const updater = setState.mock.calls[0][0];
            const result = updater([original]);
            expect(result).toEqual([updated]);
            expect(result[0].spec.replicas).toBe(3);
        });

        it('adds resource if not found (race condition fix)', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-new', 'new-resource');

            handler({ type: 'MODIFIED', resource });

            const updater = setState.mock.calls[0][0];
            const result = updater([]); // Empty list - resource not found
            expect(result).toEqual([resource]); // Should be added
        });

        it('adds resource to existing list if not found', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const existing = createMockResource('uid-1', 'existing');
            const newResource = createMockResource('uid-2', 'new');

            handler({ type: 'MODIFIED', resource: newResource });

            const updater = setState.mock.calls[0][0];
            const result = updater([existing]);
            expect(result).toEqual([existing, newResource]);
        });

        it('only updates matching resource', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource1 = createMockResource('uid-1', 'resource-1');
            const resource2 = createMockResource('uid-2', 'resource-2');
            const updatedResource1 = { ...createMockResource('uid-1', 'updated-1'), spec: {} };

            handler({ type: 'MODIFIED', resource: updatedResource1 });

            const updater = setState.mock.calls[0][0];
            const result = updater([resource1, resource2]);
            expect(result).toEqual([updatedResource1, resource2]);
        });
    });

    describe('DELETED events', () => {
        it('removes resource from list', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource1 = createMockResource('uid-1', 'resource-1');
            const resource2 = createMockResource('uid-2', 'resource-2');
            const toDelete = createMockResource('uid-1', 'resource-1');

            handler({ type: 'DELETED', resource: toDelete });

            const updater = setState.mock.calls[0][0];
            const result = updater([resource1, resource2]);
            expect(result).toEqual([resource2]);
        });

        it('handles delete of non-existent resource', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const existing = createMockResource('uid-1', 'existing');
            const toDelete = createMockResource('uid-999', 'not-found');

            handler({ type: 'DELETED', resource: toDelete });

            const updater = setState.mock.calls[0][0];
            const result = updater([existing]);
            expect(result).toEqual([existing]);
        });

        it('removes all instances with matching uid', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1', 'resource');
            const toDelete = createMockResource('uid-1', 'resource');

            handler({ type: 'DELETED', resource: toDelete });

            const updater = setState.mock.calls[0][0];
            const result = updater([resource]);
            expect(result).toEqual([]);
        });
    });

    describe('unknown event types', () => {
        it('returns prev state unchanged for unknown type', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1');

            handler({ type: 'UNKNOWN', resource });

            const updater = setState.mock.calls[0][0];
            const prev = [createMockResource('uid-2')];
            const result = updater(prev);
            expect(result).toBe(prev); // Same reference
        });
    });
});

describe('createNamespacedResourceEventHandler', () => {
    const createMockResource = (uid, name = 'test', namespace = 'default') => ({
        metadata: { uid, name, namespace }
    });

    describe('namespace filtering', () => {
        it('processes event when namespace matches selected', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, ['default']);
            const resource = createMockResource('uid-1', 'resource', 'default');

            handler({ type: 'ADDED', resource, namespace: 'default' });

            expect(setState).toHaveBeenCalled();
        });

        it('ignores event when namespace not in selected', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, ['default']);
            const resource = createMockResource('uid-1', 'resource', 'other-ns');

            handler({ type: 'ADDED', resource, namespace: 'other-ns' });

            expect(setState).not.toHaveBeenCalled();
        });

        it('processes all events when selectedNamespaces includes "*"', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, ['*']);
            const resource = createMockResource('uid-1', 'resource', 'any-namespace');

            handler({ type: 'ADDED', resource, namespace: 'any-namespace' });

            expect(setState).toHaveBeenCalled();
        });

        it('processes all events when selectedNamespaces is empty (all namespaces mode)', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, []);
            const resource = createMockResource('uid-1', 'resource', 'any-namespace');

            handler({ type: 'ADDED', resource, namespace: 'any-namespace' });

            expect(setState).toHaveBeenCalled();
        });

        it('processes event when namespace in multiple selected', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, ['ns1', 'ns2', 'ns3']);
            const resource = createMockResource('uid-1', 'resource', 'ns2');

            handler({ type: 'ADDED', resource, namespace: 'ns2' });

            expect(setState).toHaveBeenCalled();
        });
    });

    describe('MODIFIED race condition fix', () => {
        it('adds resource if MODIFIED arrives before ADDED', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, ['default']);
            const resource = createMockResource('uid-new', 'new-resource', 'default');

            handler({ type: 'MODIFIED', resource, namespace: 'default' });

            const updater = setState.mock.calls[0][0];
            const result = updater([]); // Empty list simulates MODIFIED before ADDED
            expect(result).toEqual([resource]);
        });

        it('does NOT re-add resource with deletionTimestamp after DELETE (namespaced)', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, ['default']);
            const resource = {
                metadata: { uid: 'uid-deleted', name: 'deleted-pod', namespace: 'default', deletionTimestamp: '2024-01-01T00:00:00Z' }
            };

            handler({ type: 'MODIFIED', resource, namespace: 'default' });

            const updater = setState.mock.calls[0][0];
            const result = updater([]); // Empty list = DELETE already processed
            expect(result).toEqual([]); // Should NOT re-add
        });

        it('does NOT re-add resource with deletionTimestamp after DELETE (cluster-scoped)', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = {
                metadata: { uid: 'uid-deleted', name: 'deleted-node', deletionTimestamp: '2024-01-01T00:00:00Z' }
            };

            handler({ type: 'MODIFIED', resource });

            const updater = setState.mock.calls[0][0];
            const result = updater([]); // Empty list = DELETE already processed
            expect(result).toEqual([]); // Should NOT re-add
        });
    });
});
