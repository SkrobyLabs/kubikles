import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const harness = vi.hoisted(() => ({
    states: [] as any[], refs: [] as any[], effects: [] as Array<() => any>,
    stateIndex: 0, refIndex: 0,
}));
const api = vi.hoisted(() => ({
    ListNamespacesForContext: vi.fn(), SubscribeResourceWatcher: vi.fn(), UnsubscribeWatcher: vi.fn(),
}));
const events = vi.hoisted(() => new Map<string, (event: any) => void>());
vi.mock('react', async importOriginal => ({
    ...await importOriginal<typeof import('react')>(),
    useState: (initial: any) => {
        const index = harness.stateIndex++;
        if (!(index in harness.states)) harness.states[index] = typeof initial === 'function' ? initial() : initial;
        return [harness.states[index], (next: any) => {
            harness.states[index] = typeof next === 'function' ? next(harness.states[index]) : next;
        }];
    },
    useRef: (initial: any) => harness.refs[harness.refIndex++] ||= { current: initial },
    useCallback: (callback: any) => callback,
    useMemo: (factory: any) => factory(),
    useEffect: (effect: () => any) => harness.effects.push(effect),
}));
vi.mock('wailsjs/go/main/App', () => api);
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: any) => {
    events.set(name, callback);
    return () => events.delete(name);
} }));
vi.mock('./NotificationContext', () => ({ useNotification: () => ({ addNotification: vi.fn() }) }));
vi.mock('../utils/Logger', () => ({ default: { debug: vi.fn(), info: vi.fn(), warn: vi.fn(), error: vi.fn() } }));
import { K8sProvider } from './K8sContext';

function render() {
    harness.stateIndex = 0;
    harness.refIndex = 0;
    harness.effects = [];
    return (K8sProvider({ children: null }) as any).props.value;
}
function effectContaining(text: string) {
    return harness.effects.find(effect => effect.toString().includes(text))!();
}

beforeEach(() => {
    vi.clearAllMocks();
    events.clear();
    harness.states = [];
    harness.refs = [];
    vi.stubGlobal('window', {}); // Browser/server mode has no Wails runtime.
    vi.stubGlobal('localStorage', { getItem: () => null });
    render();
    harness.states[1] = 'cluster-a';
    harness.states[8] = false; // Connected.
    harness.refs[0].current = 'cluster-a';
    api.ListNamespacesForContext.mockResolvedValue([{ metadata: { name: 'default' } }]);
    api.SubscribeResourceWatcher.mockResolvedValue('namespace-watch');
    api.UnsubscribeWatcher.mockResolvedValue(undefined);
});

afterEach(() => vi.unstubAllGlobals());

describe('namespace selector freshness', () => {
    it('refreshes the requested context and includes newly created namespaces on the next open', async () => {
        const value = render();
        await value.refreshNamespaces();
        api.ListNamespacesForContext.mockResolvedValue([{ metadata: { name: 'default' } }, { metadata: { name: 'new' } }]);
        await value.refreshNamespaces();
        expect(api.ListNamespacesForContext).toHaveBeenLastCalledWith('cluster-a');
        expect(render().namespaces).toEqual(['', 'default', 'new']);
    });

    it('ignores a list response after switching contexts', async () => {
        let finish!: (value: any) => void;
        api.ListNamespacesForContext.mockReturnValue(new Promise(resolve => { finish = resolve; }));
        const pending = render().refreshNamespaces();
        harness.refs[0].current = 'cluster-b';
        finish([{ metadata: { name: 'old-cluster' } }]);
        await pending;
        expect(render().namespaces).toEqual([]);
    });

    it('subscribes in server mode and applies single and batched events', async () => {
        render();
        const cleanup = effectContaining('resource-events-batch');
        await Promise.resolve();
        expect(api.SubscribeResourceWatcher).toHaveBeenCalledWith('namespaces', '');
        events.get('resource-event')!({ resourceType: 'namespaces', type: 'ADDED', context: 'cluster-a', resource: { metadata: { name: 'new' } } });
        expect(render().namespaces).toEqual(['', 'new']);
        events.get('resource-events-batch')!([
            { resourceType: 'namespaces', type: 'ADDED', context: 'cluster-b', resource: 'stale' },
            { resourceType: 'namespaces', type: 'DELETED', context: 'cluster-a', resource: 'new' },
        ]);
        expect(render().namespaces).toEqual(['']);
        cleanup();
        expect(api.UnsubscribeWatcher).toHaveBeenCalledWith('namespace-watch');
    });

    it('reloads namespace options after watcher reconnection', async () => {
        vi.useFakeTimers();
        try {
            render();
            const cleanup = effectContaining('watcher-error');
            const status = events.get('watcher-status')!;
            status({ resourceType: 'namespaces', status: 'reconnecting', context: 'cluster-a' });
            status({ resourceType: 'namespaces', status: 'connected', context: 'cluster-a' });
            await vi.advanceTimersByTimeAsync(500);
            render();
            effectContaining('namespaceReconcileTokenRef.current');
            await Promise.resolve();
            expect(api.ListNamespacesForContext).toHaveBeenCalledWith('cluster-a');
            expect(render().namespaces).toEqual(['', 'default']);
            cleanup();
        } finally {
            vi.useRealTimers();
        }
    });
});
