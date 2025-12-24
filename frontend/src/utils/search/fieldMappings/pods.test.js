import { describe, it, expect } from 'vitest';
import { podFields } from './pods';

describe('podFields extractors', () => {
    describe('nodename', () => {
        it('extracts spec.nodeName', () => {
            const pod = { spec: { nodeName: 'worker-1' } };
            expect(podFields.nodename.extractor(pod)).toBe('worker-1');
        });

        it('returns empty string when nodeName is missing', () => {
            expect(podFields.nodename.extractor({ spec: {} })).toBe('');
            expect(podFields.nodename.extractor({})).toBe('');
        });

        it('has alias "node"', () => {
            expect(podFields.nodename.aliases).toContain('node');
        });
    });

    describe('status', () => {
        it('returns phase for simple running pod', () => {
            const pod = { metadata: {}, status: { phase: 'Running' } };
            expect(podFields.status.extractor(pod)).toBe('Running');
        });

        it('returns Terminating when deletionTimestamp is set', () => {
            const pod = {
                metadata: { deletionTimestamp: '2024-01-15T12:00:00Z' },
                status: { phase: 'Running' }
            };
            expect(podFields.status.extractor(pod)).toBe('Terminating');
        });

        it('returns Init:Error for failed init container', () => {
            const pod = {
                metadata: {},
                status: {
                    phase: 'Pending',
                    initContainerStatuses: [{
                        state: { terminated: { exitCode: 1 } }
                    }]
                }
            };
            expect(podFields.status.extractor(pod)).toBe('Init:Error');
        });

        it('returns Init:CrashLoopBackOff for crashing init container', () => {
            const pod = {
                metadata: {},
                status: {
                    phase: 'Pending',
                    initContainerStatuses: [{
                        state: { waiting: { reason: 'CrashLoopBackOff' } }
                    }]
                }
            };
            expect(podFields.status.extractor(pod)).toBe('Init:CrashLoopBackOff');
        });

        it('returns CrashLoopBackOff from container status', () => {
            const pod = {
                metadata: {},
                status: {
                    phase: 'Running',
                    containerStatuses: [{
                        state: { waiting: { reason: 'CrashLoopBackOff' } }
                    }]
                }
            };
            expect(podFields.status.extractor(pod)).toBe('CrashLoopBackOff');
        });

        it('returns ErrImagePull from container status', () => {
            const pod = {
                metadata: {},
                status: {
                    phase: 'Pending',
                    containerStatuses: [{
                        state: { waiting: { reason: 'ErrImagePull' } }
                    }]
                }
            };
            expect(podFields.status.extractor(pod)).toBe('ErrImagePull');
        });

        it('returns ImagePullBackOff from container status', () => {
            const pod = {
                metadata: {},
                status: {
                    phase: 'Pending',
                    containerStatuses: [{
                        state: { waiting: { reason: 'ImagePullBackOff' } }
                    }]
                }
            };
            expect(podFields.status.extractor(pod)).toBe('ImagePullBackOff');
        });

        it('returns Unknown when phase is missing', () => {
            const pod = { metadata: {}, status: {} };
            expect(podFields.status.extractor(pod)).toBe('Unknown');
        });

        it('has aliases "phase" and "s"', () => {
            expect(podFields.status.aliases).toContain('phase');
            expect(podFields.status.aliases).toContain('s');
        });
    });

    describe('ip', () => {
        it('extracts status.podIP', () => {
            const pod = { status: { podIP: '10.0.0.1' } };
            expect(podFields.ip.extractor(pod)).toBe('10.0.0.1');
        });

        it('returns empty string when podIP is missing', () => {
            expect(podFields.ip.extractor({ status: {} })).toBe('');
            expect(podFields.ip.extractor({})).toBe('');
        });

        it('has alias "podip"', () => {
            expect(podFields.ip.aliases).toContain('podip');
        });
    });

    describe('hostip', () => {
        it('extracts status.hostIP', () => {
            const pod = { status: { hostIP: '192.168.1.1' } };
            expect(podFields.hostip.extractor(pod)).toBe('192.168.1.1');
        });

        it('returns empty string when hostIP is missing', () => {
            expect(podFields.hostip.extractor({ status: {} })).toBe('');
        });
    });

    describe('restarts', () => {
        it('sums restart counts from all containers', () => {
            const pod = {
                status: {
                    containerStatuses: [
                        { restartCount: 3 },
                        { restartCount: 2 }
                    ]
                }
            };
            expect(podFields.restarts.extractor(pod)).toBe('5');
        });

        it('returns "0" for no restarts', () => {
            const pod = {
                status: {
                    containerStatuses: [
                        { restartCount: 0 }
                    ]
                }
            };
            expect(podFields.restarts.extractor(pod)).toBe('0');
        });

        it('returns "0" when containerStatuses is missing', () => {
            expect(podFields.restarts.extractor({ status: {} })).toBe('0');
            expect(podFields.restarts.extractor({})).toBe('0');
        });

        it('handles containers without restartCount', () => {
            const pod = {
                status: {
                    containerStatuses: [
                        { restartCount: 5 },
                        {} // no restartCount
                    ]
                }
            };
            expect(podFields.restarts.extractor(pod)).toBe('5');
        });

        it('has alias "restart"', () => {
            expect(podFields.restarts.aliases).toContain('restart');
        });
    });

    describe('controlledby', () => {
        it('returns controller kind', () => {
            const pod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'ReplicaSet', name: 'nginx-abc123', controller: true }
                    ]
                }
            };
            expect(podFields.controlledby.extractor(pod)).toBe('ReplicaSet');
        });

        it('finds controller among multiple owners', () => {
            const pod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'ConfigMap', name: 'config' },
                        { kind: 'DaemonSet', name: 'ds-abc', controller: true }
                    ]
                }
            };
            expect(podFields.controlledby.extractor(pod)).toBe('DaemonSet');
        });

        it('returns empty string when no controller', () => {
            const pod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'ConfigMap', name: 'config' }
                    ]
                }
            };
            expect(podFields.controlledby.extractor(pod)).toBe('');
        });

        it('returns empty string when no owner references', () => {
            expect(podFields.controlledby.extractor({ metadata: {} })).toBe('');
            expect(podFields.controlledby.extractor({})).toBe('');
        });

        it('has aliases "controller" and "owner"', () => {
            expect(podFields.controlledby.aliases).toContain('controller');
            expect(podFields.controlledby.aliases).toContain('owner');
        });
    });

    describe('controllername', () => {
        it('returns controller name', () => {
            const pod = {
                metadata: {
                    ownerReferences: [
                        { kind: 'ReplicaSet', name: 'nginx-abc123', controller: true }
                    ]
                }
            };
            expect(podFields.controllername.extractor(pod)).toBe('nginx-abc123');
        });

        it('returns empty string when no controller', () => {
            expect(podFields.controllername.extractor({ metadata: {} })).toBe('');
        });

        it('has alias "ownername"', () => {
            expect(podFields.controllername.aliases).toContain('ownername');
        });
    });

    describe('container', () => {
        it('joins all container names', () => {
            const pod = {
                spec: {
                    containers: [
                        { name: 'nginx' },
                        { name: 'sidecar' }
                    ]
                }
            };
            const result = podFields.container.extractor(pod);
            expect(result).toBe('nginx sidecar');
        });

        it('includes init containers', () => {
            const pod = {
                spec: {
                    initContainers: [{ name: 'init' }],
                    containers: [{ name: 'main' }]
                }
            };
            const result = podFields.container.extractor(pod);
            expect(result).toContain('init');
            expect(result).toContain('main');
        });

        it('returns empty string when no containers', () => {
            expect(podFields.container.extractor({ spec: {} })).toBe('');
            expect(podFields.container.extractor({})).toBe('');
        });

        it('has alias "containers"', () => {
            expect(podFields.container.aliases).toContain('containers');
        });
    });

    describe('image', () => {
        it('joins all container images', () => {
            const pod = {
                spec: {
                    containers: [
                        { name: 'nginx', image: 'nginx:1.19' },
                        { name: 'sidecar', image: 'envoyproxy/envoy:v1.20' }
                    ]
                }
            };
            const result = podFields.image.extractor(pod);
            expect(result).toBe('nginx:1.19 envoyproxy/envoy:v1.20');
        });

        it('includes init container images', () => {
            const pod = {
                spec: {
                    initContainers: [{ name: 'init', image: 'busybox:latest' }],
                    containers: [{ name: 'main', image: 'nginx:1.19' }]
                }
            };
            const result = podFields.image.extractor(pod);
            expect(result).toContain('busybox:latest');
            expect(result).toContain('nginx:1.19');
        });

        it('has alias "images"', () => {
            expect(podFields.image.aliases).toContain('images');
        });
    });

    describe('serviceaccount', () => {
        it('extracts serviceAccountName', () => {
            const pod = { spec: { serviceAccountName: 'my-sa' } };
            expect(podFields.serviceaccount.extractor(pod)).toBe('my-sa');
        });

        it('falls back to legacy serviceAccount field', () => {
            const pod = { spec: { serviceAccount: 'legacy-sa' } };
            expect(podFields.serviceaccount.extractor(pod)).toBe('legacy-sa');
        });

        it('prefers serviceAccountName over serviceAccount', () => {
            const pod = { spec: { serviceAccountName: 'new-sa', serviceAccount: 'old-sa' } };
            expect(podFields.serviceaccount.extractor(pod)).toBe('new-sa');
        });

        it('returns empty string when missing', () => {
            expect(podFields.serviceaccount.extractor({ spec: {} })).toBe('');
        });

        it('has alias "sa"', () => {
            expect(podFields.serviceaccount.aliases).toContain('sa');
        });
    });

    describe('qos', () => {
        it('extracts status.qosClass', () => {
            const pod = { status: { qosClass: 'BestEffort' } };
            expect(podFields.qos.extractor(pod)).toBe('BestEffort');
        });

        it('returns empty string when missing', () => {
            expect(podFields.qos.extractor({ status: {} })).toBe('');
        });

        it('has alias "qosclass"', () => {
            expect(podFields.qos.aliases).toContain('qosclass');
        });
    });

    describe('inherited common fields', () => {
        it('has name field from common', () => {
            const pod = { metadata: { name: 'test-pod' } };
            expect(podFields.name.extractor(pod)).toBe('test-pod');
        });

        it('has namespace field from common', () => {
            const pod = { metadata: { namespace: 'default' } };
            expect(podFields.namespace.extractor(pod)).toBe('default');
        });

        it('has labels field from common', () => {
            const pod = { metadata: { labels: { app: 'test' } } };
            expect(podFields.labels.extractor(pod)).toBe('app=test');
        });
    });
});
