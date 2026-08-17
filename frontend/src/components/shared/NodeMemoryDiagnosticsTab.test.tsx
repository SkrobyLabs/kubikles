import { describe, expect, it, vi } from 'vitest';
import type { EventMarker } from './metrics/MetricsChart';

const hookState = vi.hoisted(() => ({
    effects: [] as Array<() => void | (() => void)>,
    markerUpdates: [] as EventMarker[][],
    refs: [] as Array<{ current: number }>,
    stateIndex: 0,
    refIndex: 0,
}));

const api = vi.hoisted(() => ({
    DetectPrometheus: vi.fn(),
    GetMetricsEventMarkers: vi.fn(),
    GetNodeMemoryDiagnosticsHistory: vi.fn(),
    GetNodeMemoryDiagnosticsHistoryRange: vi.fn(),
    CancelMetricsRequest: vi.fn(),
}));

vi.mock('react', () => ({
    default: {},
    useCallback: <T,>(callback: T) => callback,
    useEffect: (effect: () => void | (() => void)) => hookState.effects.push(effect),
    useMemo: <T,>(factory: () => T) => factory(),
    useRef: (initialValue: number) => hookState.refs[hookState.refIndex++] ||= { current: initialValue },
    useState: <T,>(initialValue: T) => {
        const stateIndex = hookState.stateIndex++;
        return [initialValue, (value: T) => {
            if (stateIndex === 6) hookState.markerUpdates.push(value as EventMarker[]);
        }] as const;
    },
}));

vi.mock('wailsjs/go/main/App', () => api);
vi.mock('./NodeMemoryDiagnosticView', () => ({ default: () => null }));

import NodeMemoryDiagnosticsTab from './NodeMemoryDiagnosticsTab';

function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>(nextResolve => {
        resolve = nextResolve;
    });
    return { promise, resolve };
}

function render(nodeName: string) {
    hookState.effects.length = 0;
    hookState.stateIndex = 0;
    hookState.refIndex = 0;
    NodeMemoryDiagnosticsTab({ nodeName, isStale: false });
}

describe('NodeMemoryDiagnosticsTab marker requests', () => {
    it('ignores a deferred old-node marker response after the node changes', async () => {
        const oldMarkers = deferred<EventMarker[]>();
        const newMarkers = deferred<EventMarker[]>();
        const oldMarker = { timestamp: 1, reason: 'old-node', severity: 'warning', message: 'old marker', kind: 'Node' };
        const newMarker = { timestamp: 2, reason: 'new-node', severity: 'warning', message: 'new marker', kind: 'Node' };
        api.GetMetricsEventMarkers.mockReturnValueOnce(oldMarkers.promise).mockReturnValueOnce(newMarkers.promise);

        render('node-old');
        const oldCleanup = hookState.effects[1]();
        oldCleanup?.();

        render('node-new');
        hookState.effects[1]();
        newMarkers.resolve([newMarker]);
        await newMarkers.promise;
        await Promise.resolve();

        const updatesAfterNewNode = [...hookState.markerUpdates];
        oldMarkers.resolve([oldMarker]);
        await oldMarkers.promise;
        await Promise.resolve();

        expect(api.GetMetricsEventMarkers).toHaveBeenNthCalledWith(1, '', 'node-old', 'node', '1h');
        expect(api.GetMetricsEventMarkers).toHaveBeenNthCalledWith(2, '', 'node-new', 'node', '1h');
        expect(hookState.markerUpdates).toEqual(updatesAfterNewNode);
        expect(hookState.markerUpdates[hookState.markerUpdates.length - 1]).toEqual([newMarker]);
    });
});
