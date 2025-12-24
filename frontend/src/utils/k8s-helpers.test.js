import { describe, it, expect } from 'vitest';
import {
    getPodStatus,
    getPodStatusPriority,
    getPodStatusColor,
    getContainerStatusColor,
    getEffectivePodStatus,
    getDeploymentPods,
    getPodController
} from './k8s-helpers';

describe('getPodStatus', () => {
    it('returns Terminating when deletionTimestamp is set', () => {
        const pod = {
            metadata: { deletionTimestamp: '2024-01-15T12:00:00Z' },
            status: { phase: 'Running' }
        };
        expect(getPodStatus(pod)).toBe('Terminating');
    });

    it('returns Init:Error when init container has non-zero exit code', () => {
        const pod = {
            metadata: {},
            status: {
                phase: 'Pending',
                initContainerStatuses: [{
                    state: { terminated: { exitCode: 1 } }
                }]
            }
        };
        expect(getPodStatus(pod)).toBe('Init:Error');
    });

    it('returns Init:CrashLoopBackOff when init container is crashing', () => {
        const pod = {
            metadata: {},
            status: {
                phase: 'Pending',
                initContainerStatuses: [{
                    state: { waiting: { reason: 'CrashLoopBackOff' } }
                }]
            }
        };
        expect(getPodStatus(pod)).toBe('Init:CrashLoopBackOff');
    });

    it('returns phase when no special conditions', () => {
        const pod = {
            metadata: {},
            status: { phase: 'Running' }
        };
        expect(getPodStatus(pod)).toBe('Running');
    });

    it('returns Unknown when phase is missing', () => {
        const pod = { metadata: {}, status: {} };
        expect(getPodStatus(pod)).toBe('Unknown');
    });
});

describe('getPodStatusPriority', () => {
    it('returns lowest priority for Failed', () => {
        expect(getPodStatusPriority('Failed')).toBe(1);
        expect(getPodStatusPriority('Init:Error')).toBe(1);
    });

    it('returns high priority for CrashLoopBackOff', () => {
        expect(getPodStatusPriority('CrashLoopBackOff')).toBe(2);
    });

    it('returns normal priority for Running', () => {
        expect(getPodStatusPriority('Running')).toBe(8);
    });

    it('returns highest priority for Succeeded', () => {
        expect(getPodStatusPriority('Succeeded')).toBe(9);
    });

    it('returns default priority for unknown status', () => {
        expect(getPodStatusPriority('SomeUnknownStatus')).toBe(10);
    });

    it('correctly orders priorities (lower number = more severe)', () => {
        expect(getPodStatusPriority('Failed')).toBeLessThan(getPodStatusPriority('CrashLoopBackOff'));
        expect(getPodStatusPriority('CrashLoopBackOff')).toBeLessThan(getPodStatusPriority('Pending'));
        expect(getPodStatusPriority('Pending')).toBeLessThan(getPodStatusPriority('Running'));
    });
});

describe('getPodStatusColor', () => {
    it('returns success color for Running', () => {
        expect(getPodStatusColor('Running')).toBe('text-success');
    });

    it('returns dimmed success for Succeeded', () => {
        expect(getPodStatusColor('Succeeded')).toBe('text-success/70');
    });

    it('returns warning for Pending states', () => {
        expect(getPodStatusColor('Pending')).toBe('text-warning');
        expect(getPodStatusColor('ContainerCreating')).toBe('text-warning');
        expect(getPodStatusColor('Init:Running')).toBe('text-warning');
    });

    it('returns error for Failed', () => {
        expect(getPodStatusColor('Failed')).toBe('text-error');
        expect(getPodStatusColor('Init:Error')).toBe('text-error');
    });

    it('returns red-orange for transient errors', () => {
        expect(getPodStatusColor('CrashLoopBackOff')).toBe('text-red-orange');
        expect(getPodStatusColor('ImagePullBackOff')).toBe('text-red-orange');
    });

    it('returns default for unknown status', () => {
        expect(getPodStatusColor('SomeUnknownStatus')).toBe('text-text');
    });
});

describe('getContainerStatusColor', () => {
    it('returns success for running container', () => {
        const status = { state: { running: { startedAt: '2024-01-15T12:00:00Z' } } };
        expect(getContainerStatusColor(status)).toBe('bg-success');
    });

    it('returns warning for generic waiting', () => {
        const status = { state: { waiting: { reason: 'ContainerCreating' } } };
        expect(getContainerStatusColor(status)).toBe('bg-warning');
    });

    it('returns red-orange for CrashLoopBackOff', () => {
        const status = { state: { waiting: { reason: 'CrashLoopBackOff' } } };
        expect(getContainerStatusColor(status)).toBe('bg-red-orange');
    });

    it('returns red-orange for image pull errors', () => {
        expect(getContainerStatusColor({ state: { waiting: { reason: 'ErrImagePull' } } })).toBe('bg-red-orange');
        expect(getContainerStatusColor({ state: { waiting: { reason: 'ImagePullBackOff' } } })).toBe('bg-red-orange');
    });

    it('returns dimmed success for successful termination', () => {
        const status = { state: { terminated: { exitCode: 0 } } };
        expect(getContainerStatusColor(status)).toBe('bg-success/50');
    });

    it('returns error for failed termination', () => {
        const status = { state: { terminated: { exitCode: 1 } } };
        expect(getContainerStatusColor(status)).toBe('bg-error');
    });

    it('returns gray for unknown state', () => {
        const status = { state: {} };
        expect(getContainerStatusColor(status)).toBe('bg-gray-500');
    });
});

describe('getEffectivePodStatus', () => {
    it('returns Terminating when deletionTimestamp is set', () => {
        const pod = {
            metadata: { deletionTimestamp: '2024-01-15T12:00:00Z' },
            status: { containerStatuses: [{ state: { running: {} } }] }
        };
        expect(getEffectivePodStatus(pod)).toBe('Terminating');
    });

    it('returns Running when container is running', () => {
        const pod = {
            metadata: {},
            status: {
                phase: 'Running',
                containerStatuses: [{ state: { running: { startedAt: '2024-01-15T12:00:00Z' } } }]
            }
        };
        expect(getEffectivePodStatus(pod)).toBe('Running');
    });

    it('returns worst status among containers', () => {
        const pod = {
            metadata: {},
            status: {
                phase: 'Running',
                containerStatuses: [
                    { state: { running: {} } },
                    { state: { waiting: { reason: 'CrashLoopBackOff' } } }
                ]
            }
        };
        expect(getEffectivePodStatus(pod)).toBe('CrashLoopBackOff');
    });

    it('ignores Succeeded containers when other containers exist', () => {
        const pod = {
            metadata: {},
            status: {
                phase: 'Running',
                containerStatuses: [
                    { state: { terminated: { exitCode: 0 } } }, // Succeeded - should be ignored
                    { state: { running: {} } }
                ]
            }
        };
        expect(getEffectivePodStatus(pod)).toBe('Running');
    });

    it('returns phase when no container statuses', () => {
        const pod = {
            metadata: {},
            status: { phase: 'Pending' }
        };
        expect(getEffectivePodStatus(pod)).toBe('Pending');
    });
});

describe('getDeploymentPods', () => {
    const pods = [
        { metadata: { namespace: 'default', labels: { app: 'nginx', version: 'v1' } } },
        { metadata: { namespace: 'default', labels: { app: 'nginx', version: 'v2' } } },
        { metadata: { namespace: 'default', labels: { app: 'redis' } } },
        { metadata: { namespace: 'kube-system', labels: { app: 'nginx' } } }
    ];

    it('returns pods matching selector labels', () => {
        const deployment = {
            metadata: { namespace: 'default' },
            spec: { selector: { matchLabels: { app: 'nginx' } } }
        };
        const result = getDeploymentPods(deployment, pods);
        expect(result).toHaveLength(2);
        expect(result[0].metadata.labels.app).toBe('nginx');
    });

    it('returns empty array when no selector', () => {
        const deployment = { metadata: { namespace: 'default' }, spec: {} };
        expect(getDeploymentPods(deployment, pods)).toEqual([]);
    });

    it('filters by namespace', () => {
        const deployment = {
            metadata: { namespace: 'kube-system' },
            spec: { selector: { matchLabels: { app: 'nginx' } } }
        };
        const result = getDeploymentPods(deployment, pods);
        expect(result).toHaveLength(1);
        expect(result[0].metadata.namespace).toBe('kube-system');
    });

    it('matches all selector labels', () => {
        const deployment = {
            metadata: { namespace: 'default' },
            spec: { selector: { matchLabels: { app: 'nginx', version: 'v1' } } }
        };
        const result = getDeploymentPods(deployment, pods);
        expect(result).toHaveLength(1);
        expect(result[0].metadata.labels.version).toBe('v1');
    });

    it('handles null pods array', () => {
        const deployment = {
            metadata: { namespace: 'default' },
            spec: { selector: { matchLabels: { app: 'nginx' } } }
        };
        expect(getDeploymentPods(deployment, null)).toEqual([]);
    });
});

describe('getPodController', () => {
    it('returns controller info when present', () => {
        const pod = {
            metadata: {
                ownerReferences: [
                    { kind: 'ReplicaSet', name: 'nginx-abc123', uid: 'uid-123', controller: true }
                ]
            }
        };
        expect(getPodController(pod)).toEqual({
            kind: 'ReplicaSet',
            name: 'nginx-abc123',
            uid: 'uid-123'
        });
    });

    it('returns null when no owner references', () => {
        const pod = { metadata: {} };
        expect(getPodController(pod)).toBeNull();
    });

    it('returns null when no controller owner', () => {
        const pod = {
            metadata: {
                ownerReferences: [
                    { kind: 'ConfigMap', name: 'config', uid: 'uid-456', controller: false }
                ]
            }
        };
        expect(getPodController(pod)).toBeNull();
    });

    it('finds controller among multiple owners', () => {
        const pod = {
            metadata: {
                ownerReferences: [
                    { kind: 'ConfigMap', name: 'config', uid: 'uid-456' },
                    { kind: 'ReplicaSet', name: 'nginx-abc123', uid: 'uid-123', controller: true }
                ]
            }
        };
        expect(getPodController(pod)).toEqual({
            kind: 'ReplicaSet',
            name: 'nginx-abc123',
            uid: 'uid-123'
        });
    });
});
