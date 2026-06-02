import { describe, expect, it } from 'vitest';
import { getLogContainerNames, resolveLogTargetFromPods } from './logTarget';

const pod = ({ name, phase = 'Running', initState, appState, deleting = false }) => ({
    metadata: {
        name,
        namespace: 'default',
        creationTimestamp: `2026-01-01T00:00:00Z`,
        ...(deleting ? { deletionTimestamp: '2026-01-01T00:01:00Z' } : {}),
    },
    spec: {
        initContainers: [{ name: 'init-volume' }],
        containers: [{ name: 'app' }, { name: 'sidecar' }],
    },
    status: {
        phase,
        initContainerStatuses: [{ name: 'init-volume', state: initState }],
        containerStatuses: [
            { name: 'app', state: appState },
            { name: 'sidecar', state: { terminated: { exitCode: 0 } } },
        ],
    },
});

describe('log target resolution', () => {
    it('orders running app containers before completed init containers', () => {
        const result = getLogContainerNames(pod({
            name: 'workload-1',
            initState: { terminated: { exitCode: 0 } },
            appState: { running: { startedAt: '2026-01-01T00:00:10Z' } },
        }));

        expect(result).toEqual(['app', 'sidecar', 'init-volume']);
    });

    it('selects a pod with a running container before an older terminated pod', () => {
        const oldPod = pod({
            name: 'workload-old',
            phase: 'Succeeded',
            initState: { terminated: { exitCode: 0 } },
            appState: { terminated: { exitCode: 0 } },
        });
        const runningPod = pod({
            name: 'workload-running',
            initState: { terminated: { exitCode: 0 } },
            appState: { running: { startedAt: '2026-01-01T00:00:10Z' } },
        });

        const result = resolveLogTargetFromPods('default', [oldPod, runningPod], 'workload');

        expect(result?.pod).toBe('workload-running');
        expect(result?.containers[0]).toBe('app');
        expect(result?.siblingPods).toEqual(['workload-running', 'workload-old']);
    });

    it('prefers a non-deleting pod over a deleting pod with running containers', () => {
        const deletingPod = pod({
            name: 'workload-deleting',
            deleting: true,
            initState: { terminated: { exitCode: 0 } },
            appState: { running: { startedAt: '2026-01-01T00:00:10Z' } },
        });
        const replacementPod = pod({
            name: 'workload-replacement',
            phase: 'Pending',
            initState: { waiting: { reason: 'PodInitializing' } },
            appState: { waiting: { reason: 'ContainerCreating' } },
        });

        const result = resolveLogTargetFromPods('default', [deletingPod, replacementPod], 'workload');

        expect(result?.pod).toBe('workload-replacement');
        expect(result?.siblingPods).toEqual(['workload-replacement', 'workload-deleting']);
    });

    it('uses fresh running pods before stale terminated pods for container fallback', () => {
        const stalePod = pod({
            name: 'workload-stale',
            phase: 'Succeeded',
            initState: { terminated: { exitCode: 0 } },
            appState: { terminated: { exitCode: 0 } },
        });
        const freshPod = pod({
            name: 'workload-fresh',
            initState: { terminated: { exitCode: 0 } },
            appState: { running: { startedAt: '2026-01-01T00:00:10Z' } },
        });

        const result = resolveLogTargetFromPods('default', [stalePod, freshPod], 'workload');

        expect(result?.pod).toBe('workload-fresh');
        expect(result?.containers).toEqual(['app', 'sidecar', 'init-volume']);
        expect(result?.podContainerMap['workload-stale']).toEqual(['app', 'sidecar', 'init-volume']);
        expect(result?.podContainerMap['workload-fresh']).toEqual(['app', 'sidecar', 'init-volume']);
    });
});
