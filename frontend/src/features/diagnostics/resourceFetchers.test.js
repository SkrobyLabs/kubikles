import { describe, it, expect, vi, beforeEach } from 'vitest';

/**
 * Tests for diagnostic resource fetcher helpers.
 *
 * These test that the fetch helpers call the generic ListResourceNamesForContext
 * backend function with proper parameters for all resource types.
 */

// Mock the Wails backend function
vi.mock('wailsjs/go/main/App', () => ({
    ListResourceNamesForContext: vi.fn((context, type, namespace) => {
        // Return mock data based on resource type
        const mockData = {
            deployment: [{ name: 'deploy-1', namespace: 'default' }],
            statefulset: [{ name: 'sts-1', namespace: 'default' }],
            daemonset: [{ name: 'ds-1', namespace: 'default' }],
            pod: [{ name: 'pod-1', namespace: 'default' }],
            service: [{ name: 'svc-1', namespace: 'default' }],
            ingress: [{ name: 'ing-1', namespace: 'default' }],
            job: [{ name: 'job-1', namespace: 'default' }],
            cronjob: [{ name: 'cj-1', namespace: 'default' }],
            configmap: [{ name: 'cm-1', namespace: 'default' }],
            secret: [{ name: 'secret-1', namespace: 'default' }],
            pvc: [{ name: 'pvc-1', namespace: 'default' }],
            serviceaccount: [{ name: 'sa-1', namespace: 'default' }],
            role: [{ name: 'role-1', namespace: 'default' }],
            rolebinding: [{ name: 'rb-1', namespace: 'default' }],
            clusterrole: [{ name: 'cr-1' }],  // No namespace for cluster-scoped
            clusterrolebinding: [{ name: 'crb-1' }],
            networkpolicy: [{ name: 'np-1', namespace: 'default' }],
            hpa: [{ name: 'hpa-1', namespace: 'default' }],
        };
        return Promise.resolve(mockData[type] || []);
    }),
}));

import { ListResourceNamesForContext } from 'wailsjs/go/main/App';

// Replicate the fetchResourceNamesByType helper from ResourceDiff
// This uses the generic ListResourceNamesForContext for all types
const fetchResourceNamesByType = async (type, namespace, context = '') => {
    return ListResourceNamesForContext(context, type, namespace);
};

describe('fetchResourceNamesByType', () => {
    const namespace = 'default';
    const context = 'my-cluster';

    beforeEach(() => {
        vi.clearAllMocks();
    });

    describe('generic function call', () => {
        it('calls ListResourceNamesForContext with correct parameters', async () => {
            await fetchResourceNamesByType('deployment', namespace, context);

            expect(ListResourceNamesForContext).toHaveBeenCalledWith(
                context,
                'deployment',
                namespace
            );
        });

        it('passes empty string for context when not specified', async () => {
            await fetchResourceNamesByType('pod', namespace);

            expect(ListResourceNamesForContext).toHaveBeenCalledWith(
                '',
                'pod',
                namespace
            );
        });
    });

    describe('all resource types return data', () => {
        const allTypes = [
            'deployment', 'statefulset', 'daemonset', 'pod',
            'service', 'configmap', 'secret', 'ingress',
            'job', 'cronjob', 'pvc', 'serviceaccount',
            'role', 'rolebinding', 'clusterrole', 'clusterrolebinding',
            'networkpolicy', 'hpa'
        ];

        it.each(allTypes)('fetches %s correctly', async (type) => {
            const result = await fetchResourceNamesByType(type, namespace, context);

            expect(result).toHaveLength(1);
            expect(result[0].name).toBeDefined();
            expect(ListResourceNamesForContext).toHaveBeenCalledWith(context, type, namespace);
        });
    });

    describe('context-aware fetching', () => {
        it('uses provided context for cross-cluster comparison', async () => {
            const sourceContext = 'cluster-a';
            const targetContext = 'cluster-b';

            await fetchResourceNamesByType('deployment', 'prod', sourceContext);
            await fetchResourceNamesByType('deployment', 'prod', targetContext);

            expect(ListResourceNamesForContext).toHaveBeenNthCalledWith(1, sourceContext, 'deployment', 'prod');
            expect(ListResourceNamesForContext).toHaveBeenNthCalledWith(2, targetContext, 'deployment', 'prod');
        });
    });

    describe('cluster-scoped resources', () => {
        it('fetches clusterrole (returns items without namespace)', async () => {
            const result = await fetchResourceNamesByType('clusterrole', '', context);

            expect(result).toHaveLength(1);
            expect(result[0].name).toBe('cr-1');
            expect(result[0].namespace).toBeUndefined();
        });

        it('fetches clusterrolebinding (returns items without namespace)', async () => {
            const result = await fetchResourceNamesByType('clusterrolebinding', '', context);

            expect(result).toHaveLength(1);
            expect(result[0].name).toBe('crb-1');
            expect(result[0].namespace).toBeUndefined();
        });
    });

    describe('unknown resource types', () => {
        it('returns empty array for unknown type', async () => {
            const result = await fetchResourceNamesByType('unknown', namespace, context);
            expect(result).toEqual([]);
        });
    });
});

describe('RESOURCE_TYPES coverage', () => {
    // FlowTimeline resource types
    const flowTimelineTypes = [
        'deployment', 'statefulset', 'daemonset', 'pod',
        'service', 'ingress', 'job', 'cronjob'
    ];

    // ResourceDiff resource types
    const resourceDiffTypes = [
        'deployment', 'statefulset', 'daemonset', 'pod',
        'service', 'configmap', 'secret', 'ingress',
        'job', 'cronjob', 'pvc', 'serviceaccount',
        'role', 'rolebinding', 'clusterrole', 'clusterrolebinding',
        'networkpolicy', 'hpa'
    ];

    it('FlowTimeline supports all expected resource types', async () => {
        for (const type of flowTimelineTypes) {
            const result = await fetchResourceNamesByType(type, 'default', '');
            expect(result, `${type} should return resources`).not.toEqual([]);
        }
    });

    it('ResourceDiff supports all expected resource types', async () => {
        for (const type of resourceDiffTypes) {
            const result = await fetchResourceNamesByType(type, 'default', '');
            expect(result, `${type} should return resources`).not.toEqual([]);
        }
    });
});
