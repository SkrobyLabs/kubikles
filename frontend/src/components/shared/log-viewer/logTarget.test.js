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
});
