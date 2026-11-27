import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const harness = vi.hoisted(() => ({
    states: [] as any[], refs: [] as any[], effects: [] as Array<() => any>,
    stateIndex: 0, refIndex: 0,
}));
const api = vi.hoisted(() => ({
    SwitchContext: vi.fn(), ListNamespacesForContext: vi.fn(), StartAutoStartPortForwards: vi.fn().mockResolvedValue(undefined), SubscribeResourceWatcher: vi.fn(), UnsubscribeWatcher: vi.fn(),
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


function renderTree() {
    harness.stateIndex = 0;
    harness.refIndex = 0;
    harness.effects = [];
    return K8sProvider({ children: null }) as any;
}

describe('useK8sActions value', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        events.clear();
        harness.states = [];
        harness.refs = [];
        vi.stubGlobal('window', {});
        vi.stubGlobal('localStorage', { getItem: () => null, setItem: vi.fn() });
    });
    afterEach(() => { vi.unstubAllGlobals(); });

    it('exposes only functions, and wrappers forward to the latest implementation', () => {
        const tree = renderTree();
        const full = tree.props.value;
        const actions = tree.props.children.props.value;
        for (const key of Object.keys(actions)) expect(typeof actions[key]).toBe('function');
        expect(Object.keys(actions)).toContain('refreshNamespaces');
        expect(Object.keys(actions)).not.toContain('namespaces');
        const spy = vi.fn().mockReturnValue('ok');
        full.triggerRefresh = spy;
        expect(actions.triggerRefresh('a', 1)).toBe('ok');
        expect(spy).toHaveBeenCalledWith('a', 1);
    });
});
