import { describe, it, expect } from 'vitest';
import { getOwnerKey, getTotalRestarts } from './usePodNotifications';

describe('usePodNotifications helpers', () => {
    describe('getOwnerKey', () => {
        it('returns null for null/undefined pod', () => {
            expect(getOwnerKey(null)).toBeNull();
            expect(getOwnerKey(undefined)).toBeNull();
        });

        it('returns null for pod with no metadata', () => {
            expect(getOwnerKey({})).toBeNull();
            expect(getOwnerKey({ status: {} })).toBeNull();
        });

        it('returns null for pod with no ownerReferences', () => {
            expect(getOwnerKey({ metadata: {} })).toBeNull();
            expect(getOwnerKey({ metadata: { ownerReferences: [] } })).toBeNull();
        });

        it('returns kind/name for first owner', () => {
            const pod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'ReplicaSet', name: 'nginx-abc123' }
                    ]
                }
            };
            expect(getOwnerKey(pod)).toBe('ReplicaSet/nginx-abc123');
        });

        it('returns only first owner when multiple exist', () => {
            const pod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'ReplicaSet', name: 'first-owner' },
                        { kind: 'Deployment', name: 'second-owner' }
                    ]
                }
            };
            expect(getOwnerKey(pod)).toBe('ReplicaSet/first-owner');
        });

        it('handles different owner kinds', () => {
            const statefulSetPod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'StatefulSet', name: 'mysql-0' }
                    ]
                }
            };
            expect(getOwnerKey(statefulSetPod)).toBe('StatefulSet/mysql-0');

            const daemonSetPod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'DaemonSet', name: 'fluentd' }
                    ]
                }
            };
            expect(getOwnerKey(daemonSetPod)).toBe('DaemonSet/fluentd');

            const jobPod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'Job', name: 'batch-job-xyz' }
                    ]
                }
            };
            expect(getOwnerKey(jobPod)).toBe('Job/batch-job-xyz');
        });
    });

    describe('getTotalRestarts', () => {
        it('returns 0 for null/undefined pod', () => {
            expect(getTotalRestarts(null)).toBe(0);
            expect(getTotalRestarts(undefined)).toBe(0);
        });

        it('returns 0 for pod with no status', () => {
            expect(getTotalRestarts({})).toBe(0);
            expect(getTotalRestarts({ metadata: {} })).toBe(0);
        });

        it('returns 0 for pod with no containerStatuses', () => {
            expect(getTotalRestarts({ status: {} })).toBe(0);
            expect(getTotalRestarts({ status: { containerStatuses: null } })).toBe(0);
        });

        it('returns 0 for pod with empty containerStatuses', () => {
            expect(getTotalRestarts({ status: { containerStatuses: [] } })).toBe(0);
        });

        it('returns single container restart count', () => {
            const pod = {
                status: {
                    containerStatuses: [
                        { name: 'main', restartCount: 5 }
                    ]
                }
            };
            expect(getTotalRestarts(pod)).toBe(5);
        });

        it('sums restart counts across multiple containers', () => {
            const pod = {
                status: {
                    containerStatuses: [
                        { name: 'main', restartCount: 3 },
                        { name: 'sidecar', restartCount: 2 },
                        { name: 'init', restartCount: 1 }
                    ]
                }
            };
            expect(getTotalRestarts(pod)).toBe(6);
        });

        it('handles containers with zero restarts', () => {
            const pod = {
                status: {
                    containerStatuses: [
                        { name: 'stable', restartCount: 0 },
                        { name: 'flaky', restartCount: 10 }
                    ]
                }
            };
            expect(getTotalRestarts(pod)).toBe(10);
        });

        it('handles missing restartCount as 0', () => {
            const pod = {
                status: {
                    containerStatuses: [
                        { name: 'no-restart-field' },
                        { name: 'has-restarts', restartCount: 3 }
                    ]
                }
            };
            expect(getTotalRestarts(pod)).toBe(3);
        });

        it('handles realistic pod structure', () => {
            const pod = {
                metadata: {
                    name: 'web-server-abc123',
                    namespace: 'production'
                },
                status: {
                    phase: 'Running',
                    containerStatuses: [
                        {
                            name: 'nginx',
                            state: { running: { startedAt: '2024-01-01T00:00:00Z' } },
                            restartCount: 2,
                            ready: true
                        },
                        {
                            name: 'log-collector',
                            state: { running: { startedAt: '2024-01-01T00:00:00Z' } },
                            restartCount: 0,
                            ready: true
                        }
                    ]
                }
            };
            expect(getTotalRestarts(pod)).toBe(2);
        });
    });
});
