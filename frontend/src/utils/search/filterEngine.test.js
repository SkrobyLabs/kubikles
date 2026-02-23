import { describe, it, expect } from 'vitest';
import { createFilter } from './filterEngine';

// Mock pod data for testing
const mockPods = [
    {
        metadata: { name: 'nginx-abc123', namespace: 'default', labels: { app: 'nginx' } },
        spec: { nodeName: 'worker-1' },
        status: { phase: 'Running', podIP: '10.0.0.1' }
    },
    {
        metadata: { name: 'nginx-def456', namespace: 'default', labels: { app: 'nginx' } },
        spec: { nodeName: 'worker-2' },
        status: { phase: 'Pending', podIP: '10.0.0.2' }
    },
    {
        metadata: { name: 'redis-xyz789', namespace: 'kube-system', labels: { app: 'redis' } },
        spec: { nodeName: 'worker-1' },
        status: { phase: 'Running', podIP: '10.0.0.3' }
    },
    {
        metadata: { name: 'api-server', namespace: 'production', labels: { app: 'api' } },
        spec: { nodeName: 'worker-3' },
        status: { phase: 'Running', podIP: '10.0.0.4' }
    }
];

describe('createFilter', () => {
    describe('empty query', () => {
        it('returns match-all function for empty string', () => {
            const filter = createFilter('pods', '');
            expect(mockPods.every(filter)).toBe(true);
        });

        it('returns match-all function for whitespace', () => {
            const filter = createFilter('pods', '   ');
            expect(mockPods.every(filter)).toBe(true);
        });

        it('returns match-all function for null/undefined', () => {
            const filter1 = createFilter('pods', null);
            const filter2 = createFilter('pods', undefined);
            expect(mockPods.every(filter1)).toBe(true);
            expect(mockPods.every(filter2)).toBe(true);
        });
    });

    describe('plain text queries', () => {
        it('matches name substring case-insensitively', () => {
            const filter = createFilter('pods', 'nginx');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(2);
            expect(results.every(p => p.metadata.name.includes('nginx'))).toBe(true);
        });

        it('matches partial name', () => {
            const filter = createFilter('pods', 'api');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(1);
            expect(results[0].metadata.name).toBe('api-server');
        });

        it('returns empty for no matches', () => {
            const filter = createFilter('pods', 'nonexistent');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(0);
        });

        it('is case insensitive', () => {
            const filter = createFilter('pods', 'NGINX');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(2);
        });
    });

    describe('field-specific queries', () => {
        it('filters by namespace field', () => {
            const filter = createFilter('pods', 'namespace:kube-system');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(1);
            expect(results[0].metadata.namespace).toBe('kube-system');
        });

        it('filters by name field', () => {
            const filter = createFilter('pods', 'name:redis');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(1);
            expect(results[0].metadata.name).toBe('redis-xyz789');
        });

        it('filters by nodename field', () => {
            const filter = createFilter('pods', 'nodename:worker-1');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(2);
        });

        it('filters by status field', () => {
            const filter = createFilter('pods', 'status:Running');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(3);
        });

        it('handles field aliases', () => {
            const filter = createFilter('pods', 'ns:default');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(2);
        });

        it('returns no matches for unknown field', () => {
            const filter = createFilter('pods', 'unknownfield:value');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(0);
        });
    });

    describe('regex queries', () => {
        it('filters with regex pattern', () => {
            const filter = createFilter('pods', 'name:/^nginx/');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(2);
        });

        it('filters with case-insensitive regex', () => {
            const filter = createFilter('pods', 'name:/NGINX/i');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(2);
        });

        it('filters with alternation regex', () => {
            const filter = createFilter('pods', 'status:/Running|Pending/');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(4);
        });

        it('filters with complex regex', () => {
            const filter = createFilter('pods', 'name:/^(nginx|redis)-[a-z]+\\d+$/');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(3); // nginx-abc123, nginx-def456, redis-xyz789
        });
    });

    describe('multiple conditions (AND)', () => {
        it('ANDs multiple plain conditions', () => {
            // This won't match anything since a pod name can't contain both
            const filter = createFilter('pods', 'nginx redis');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(0);
        });

        it('ANDs field conditions', () => {
            const filter = createFilter('pods', 'namespace:default status:Running');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(1);
            expect(results[0].metadata.name).toBe('nginx-abc123');
        });

        it('ANDs plain and field conditions', () => {
            const filter = createFilter('pods', 'nginx namespace:default');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(2);
        });

        it('ANDs field and regex conditions', () => {
            const filter = createFilter('pods', 'namespace:default name:/abc/');
            const results = mockPods.filter(filter);
            expect(results).toHaveLength(1);
            expect(results[0].metadata.name).toBe('nginx-abc123');
        });
    });
});

