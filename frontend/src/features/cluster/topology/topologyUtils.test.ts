import { describe, expect, it } from 'vitest';

import { getPodResourceRequests } from './topologyUtils';

describe('getPodResourceRequests', () => {
    it('includes pod-level requests and overhead', () => {
        const pod = {
            spec: {
                containers: [{ name: 'main', resources: {} }],
                resources: {
                    requests: {
                        cpu: '2',
                        memory: '1Gi',
                    },
                },
                overhead: {
                    cpu: '50m',
                    memory: '10Mi',
                },
            },
        };

        expect(getPodResourceRequests(pod)).toEqual({
            cpuMillis: 2050,
            memBytes: 1034 * 1024 ** 2,
        });
    });

    it('uses the max of pod, container, and init container requests', () => {
        const pod = {
            spec: {
                containers: [
                    { name: 'main', resources: { requests: { cpu: '400m', memory: '128Mi' } } },
                    { name: 'sidecar', resources: { requests: { cpu: '300m', memory: '128Mi' } } },
                ],
                initContainers: [
                    { name: 'init', resources: { requests: { cpu: '900m', memory: '64Mi' } } },
                ],
                resources: {
                    requests: {
                        cpu: '500m',
                        memory: '512Mi',
                    },
                },
            },
        };

        expect(getPodResourceRequests(pod)).toEqual({
            cpuMillis: 900,
            memBytes: 512 * 1024 ** 2,
        });
    });

    it('adds restartable init sidecars to app container requests', () => {
        const pod = {
            spec: {
                containers: [
                    { name: 'main', resources: { requests: { cpu: '500m', memory: '128Mi' } } },
                ],
                initContainers: [
                    { name: 'log-sidecar', restartPolicy: 'Always', resources: { requests: { cpu: '200m', memory: '64Mi' } } },
                ],
            },
        };

        expect(getPodResourceRequests(pod)).toEqual({
            cpuMillis: 700,
            memBytes: 192 * 1024 ** 2,
        });
    });

    it('includes earlier sidecars with later init container requests', () => {
        const pod = {
            spec: {
                containers: [
                    { name: 'main', resources: { requests: { cpu: '500m', memory: '128Mi' } } },
                ],
                initContainers: [
                    { name: 'log-sidecar', restartPolicy: 'Always', resources: { requests: { cpu: '200m', memory: '64Mi' } } },
                    { name: 'setup', resources: { requests: { cpu: '800m', memory: '256Mi' } } },
                ],
            },
        };

        expect(getPodResourceRequests(pod)).toEqual({
            cpuMillis: 1000,
            memBytes: 320 * 1024 ** 2,
        });
    });
});
