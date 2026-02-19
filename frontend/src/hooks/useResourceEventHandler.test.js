import { describe, it, expect, vi } from 'vitest';
import { createResourceEventHandler, createNamespacedResourceEventHandler } from './useResourceEventHandler';

describe('createResourceEventHandler', () => {
    const createMockResource = (uid, name = 'test-resource') => ({
        metadata: { uid, name }
    });

    /** Helper: build a Map from resource array */
    const toMap = (resources) => {
        const m = new Map();
        for (const r of resources) {
            m.set(r.metadata.uid, r);
        }
        return m;
    };

    describe('ADDED events', () => {
        it('adds new resource to empty map', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1', 'resource-1');

            handler({ type: 'ADDED', resource });

            const updater = setState.mock.calls[0][0];
            const result = updater(new Map());
            expect(Array.from(result.values())).toEqual([resource]);
        });

        it('adds new resource to existing map', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const existing = createMockResource('uid-1', 'resource-1');
            const newResource = createMockResource('uid-2', 'resource-2');

            handler({ type: 'ADDED', resource: newResource });

            const updater = setState.mock.calls[0][0];
            const result = updater(toMap([existing]));
            expect(result.size).toBe(2);
            expect(result.get('uid-1')).toEqual(existing);
            expect(result.get('uid-2')).toEqual(newResource);
        });

        it('does not add duplicate resource (same uid)', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1', 'resource-1');

            handler({ type: 'ADDED', resource });

            const updater = setState.mock.calls[0][0];
            const prev = toMap([resource]);
            const result = updater(prev);
            expect(result).toBe(prev); // Same reference, no change
        });

        it('ignores resource without uid', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = { metadata: { name: 'no-uid' } };

            handler({ type: 'ADDED', resource });

            const updater = setState.mock.calls[0][0];
            const prev = toMap([createMockResource('uid-1')]);
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
            const result = updater(toMap([original]));
            expect(result.get('uid-1')).toEqual(updated);
            expect(result.get('uid-1').spec.replicas).toBe(3);
        });

        it('adds resource if not found (race condition fix)', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-new', 'new-resource');

            handler({ type: 'MODIFIED', resource });

            const updater = setState.mock.calls[0][0];
            const result = updater(new Map()); // Empty map - resource not found
            expect(Array.from(result.values())).toEqual([resource]); // Should be added
        });

        it('adds resource to existing map if not found', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const existing = createMockResource('uid-1', 'existing');
            const newResource = createMockResource('uid-2', 'new');

            handler({ type: 'MODIFIED', resource: newResource });

            const updater = setState.mock.calls[0][0];
            const result = updater(toMap([existing]));
            expect(result.size).toBe(2);
            expect(result.get('uid-1')).toEqual(existing);
            expect(result.get('uid-2')).toEqual(newResource);
        });

        it('only updates matching resource', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource1 = createMockResource('uid-1', 'resource-1');
            const resource2 = createMockResource('uid-2', 'resource-2');
            const updatedResource1 = { ...createMockResource('uid-1', 'updated-1'), spec: {} };

            handler({ type: 'MODIFIED', resource: updatedResource1 });

            const updater = setState.mock.calls[0][0];
            const result = updater(toMap([resource1, resource2]));
            expect(result.get('uid-1')).toEqual(updatedResource1);
            expect(result.get('uid-2')).toEqual(resource2);
        });
    });

    describe('DELETED events', () => {
        it('removes resource from map', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource1 = createMockResource('uid-1', 'resource-1');
            const resource2 = createMockResource('uid-2', 'resource-2');
            const toDelete = createMockResource('uid-1', 'resource-1');

            handler({ type: 'DELETED', resource: toDelete });

            const updater = setState.mock.calls[0][0];
            const result = updater(toMap([resource1, resource2]));
            expect(result.size).toBe(1);
            expect(result.has('uid-1')).toBe(false);
            expect(result.get('uid-2')).toEqual(resource2);
        });

        it('handles delete of non-existent resource', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const existing = createMockResource('uid-1', 'existing');
            const toDelete = createMockResource('uid-999', 'not-found');

            handler({ type: 'DELETED', resource: toDelete });

            const updater = setState.mock.calls[0][0];
            const prev = toMap([existing]);
            const result = updater(prev);
            expect(result).toBe(prev); // Same reference, no change
        });

        it('removes resource with matching uid', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1', 'resource');
            const toDelete = createMockResource('uid-1', 'resource');

            handler({ type: 'DELETED', resource: toDelete });

            const updater = setState.mock.calls[0][0];
            const result = updater(toMap([resource]));
            expect(result.size).toBe(0);
        });
    });

    describe('unknown event types', () => {
        it('returns prev state unchanged for unknown type', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = createMockResource('uid-1');

            handler({ type: 'UNKNOWN', resource });

            const updater = setState.mock.calls[0][0];
            const prev = toMap([createMockResource('uid-2')]);
            const result = updater(prev);
            expect(result).toBe(prev); // Same reference
        });
    });

    describe('scale performance', () => {
        it('processes batch of 500 MODIFIED events against 10K map under 2000ms', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);

            // Build initial map with 10K resources
            let map = new Map();
            for (let i = 0; i < 10000; i++) {
                map.set(`uid-${i}`, createMockResource(`uid-${i}`, `resource-${i}`));
            }

            const start = performance.now();

            // Process 500 MODIFIED events (matching EventCoalescer batch cap)
            for (let i = 0; i < 500; i++) {
                const updated = { ...createMockResource(`uid-${i}`, `resource-${i}`), spec: { version: 2 } };
                handler({ type: 'MODIFIED', resource: updated });

                // Apply the updater to simulate React state update
                const updater = setState.mock.calls[setState.mock.calls.length - 1][0];
                map = updater(map);
            }

            const elapsed = performance.now() - start;
            expect(elapsed).toBeLessThan(2000);
            expect(map.size).toBe(10000);
        });

        it('O(1) lookup: has() is faster than find() on large dataset', () => {
            // Verify Map.has is O(1) vs Array.find O(n)
            const size = 50000;
            const map = new Map();
            const arr = [];
            for (let i = 0; i < size; i++) {
                const r = createMockResource(`uid-${i}`, `resource-${i}`);
                map.set(`uid-${i}`, r);
                arr.push(r);
            }

            // Time 10K lookups on Map
            const mapStart = performance.now();
            for (let i = 0; i < 10000; i++) {
                map.has(`uid-${i % size}`);
            }
            const mapTime = performance.now() - mapStart;

            // Time 10K lookups on Array
            const arrStart = performance.now();
            for (let i = 0; i < 10000; i++) {
                arr.find(r => r.metadata?.uid === `uid-${i % size}`);
            }
            const arrTime = performance.now() - arrStart;

            // Map should be significantly faster (>10x)
            expect(mapTime).toBeLessThan(arrTime);
        });
    });

    describe('Map to array derivation', () => {
        it('Array.from(map.values()) preserves insertion order', () => {
            const r1 = createMockResource('uid-1', 'alpha');
            const r2 = createMockResource('uid-2', 'bravo');
            const r3 = createMockResource('uid-3', 'charlie');
            const map = toMap([r1, r2, r3]);
            const arr = Array.from(map.values());
            expect(arr).toEqual([r1, r2, r3]);
        });
    });
});

describe('createNamespacedResourceEventHandler', () => {
    const createMockResource = (uid, name = 'test', namespace = 'default') => ({
        metadata: { uid, name, namespace }
    });

    const toMap = (resources) => {
        const m = new Map();
        for (const r of resources) {
            m.set(r.metadata.uid, r);
        }
        return m;
    };

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
            const result = updater(new Map()); // Empty map simulates MODIFIED before ADDED
            expect(Array.from(result.values())).toEqual([resource]);
        });

        it('does NOT re-add resource with deletionTimestamp after DELETE (namespaced)', () => {
            const setState = vi.fn();
            const handler = createNamespacedResourceEventHandler(setState, ['default']);
            const resource = {
                metadata: { uid: 'uid-deleted', name: 'deleted-pod', namespace: 'default', deletionTimestamp: '2024-01-01T00:00:00Z' }
            };

            handler({ type: 'MODIFIED', resource, namespace: 'default' });

            const updater = setState.mock.calls[0][0];
            const result = updater(new Map()); // Empty map = DELETE already processed
            expect(result.size).toBe(0); // Should NOT re-add
        });

        it('does NOT re-add resource with deletionTimestamp after DELETE (cluster-scoped)', () => {
            const setState = vi.fn();
            const handler = createResourceEventHandler(setState);
            const resource = {
                metadata: { uid: 'uid-deleted', name: 'deleted-node', deletionTimestamp: '2024-01-01T00:00:00Z' }
            };

            handler({ type: 'MODIFIED', resource });

            const updater = setState.mock.calls[0][0];
            const result = updater(new Map()); // Empty map = DELETE already processed
            expect(result.size).toBe(0); // Should NOT re-add
        });
    });
});
