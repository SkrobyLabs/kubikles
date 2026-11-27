import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Metrics hooks must not call the backend while the window is hidden: the
// first fetch happens when it becomes visible. React is replaced by a tiny
// synchronous runner (as in useResource.listwatch.test.ts) and the real
// windowVisibility module runs against a fake document.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void> }));
const mocks = vi.hoisted(() => ({
    podMetrics: vi.fn(), nodeMetrics: vi.fn(), detect: vi.fn(),
    context: { currentContext: 'ctx' } as any,
    config: {} as Record<string, any>,
}));

vi.mock('react', () => {
    const memo = (factory: () => any, deps: any[]) => {
        const index = runner.index++;
        const old = runner.slots[index];
        if (!old || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) runner.slots[index] = { value: factory(), deps };
        return runner.slots[index].value;
    };
    return {
        useMemo: memo,
        useCallback: (fn: any, deps: any[]) => memo(() => fn, deps),
        useRef: (initial: any) => memo(() => ({ current: initial }), []),
        useState: (initial: any) => {
            const index = runner.index++;
            if (!(index in runner.slots)) {
                const slot = { value: typeof initial === 'function' ? initial() : initial, set: (next: any) => {
                    slot.value = typeof next === 'function' ? next(slot.value) : next;
                } };
                runner.slots[index] = slot;
            }
            return [runner.slots[index].value, runner.slots[index].set];
        },
        useEffect: (effect: () => any, deps: any[]) => {
            const index = runner.index++;
            const old = runner.slots[index];
            if (!old || !deps || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) {
                const slot = { deps, cleanup: undefined as any };
                runner.slots[index] = slot;
                runner.effects.push(() => { old?.cleanup?.(); slot.cleanup = effect(); });
            }
        },
    };
});
vi.mock('../context', () => ({
    useK8s: () => mocks.context,
    useConfig: () => ({ getConfig: (key: string) => mocks.config[key] }),
}));
vi.mock('wailsjs/go/main/App', () => ({
    GetPodMetrics: mocks.podMetrics, GetPodMetricsFromPrometheus: vi.fn(),
    GetNodeMetrics: mocks.nodeMetrics, GetNodeMetricsFromPrometheus: vi.fn(),
    DetectPrometheus: mocks.detect,
}));
vi.mock('wailsjs/go/models', () => ({ k8s: {} }));

type Handler = () => void;
function installDom(visible: boolean) {
    const handlers: Record<string, Set<Handler>> = {};
    const add = (t: string, h: Handler) => { (handlers[t] ||= new Set()).add(h); };
    const remove = (t: string, h: Handler) => { handlers[t]?.delete(h); };
    const doc: any = { visibilityState: visible ? 'visible' : 'hidden', addEventListener: add, removeEventListener: remove };
    (globalThis as any).document = doc;
    (globalThis as any).window = { addEventListener: add, removeEventListener: remove };
    return {
        set(v: boolean) {
            doc.visibilityState = v ? 'visible' : 'hidden';
            handlers['visibilitychange']?.forEach(h => h());
        },
    };
}

function render(hook: () => any) {
    runner.index = 0; runner.effects = [];
    const result = hook();
    runner.effects.forEach(effect => effect());
    return result;
}

const flush = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };

beforeEach(() => {
    vi.resetModules();
    vi.useFakeTimers();
    runner.slots.forEach(slot => slot.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = [];
    vi.clearAllMocks();
    mocks.podMetrics.mockResolvedValue({ available: true, metrics: [] });
    mocks.nodeMetrics.mockResolvedValue({ available: true, metrics: [] });
    mocks.config = { 'kubernetes.metricsPollIntervalMs': 30000, 'metrics.preferredSource': 'k8s' };
});

afterEach(() => {
    runner.slots.forEach(slot => slot.cleanup?.());
    vi.useRealTimers();
    delete (globalThis as any).document;
    delete (globalThis as any).window;
});

const cases = [
    { name: 'usePodMetrics', path: './usePodMetrics', hookName: 'usePodMetrics', fn: () => mocks.podMetrics },
    { name: 'useNodeMetrics', path: './useNodeMetrics', hookName: 'useNodeMetrics', fn: () => mocks.nodeMetrics },
];

describe.each(cases)('$name visibility gate', ({ path, hookName, fn }) => {
    const load = async () => (await import(/* @vite-ignore */ path))[hookName] as (v: boolean, r: boolean, p: boolean) => any;

    it('fetches immediately when the window is visible', async () => {
        installDom(true);
        const hook = await load();
        render(() => hook(true, true, true));
        await flush();
        expect(fn()).toHaveBeenCalledTimes(1);
    });

    it('does not fetch while hidden and fetches once when it becomes visible', async () => {
        const dom = installDom(false);
        const hook = await load();
        render(() => hook(true, true, true));
        await flush();
        expect(fn()).not.toHaveBeenCalled();
        vi.advanceTimersByTime(120_000);
        await flush();
        expect(fn()).not.toHaveBeenCalled();
        dom.set(true);
        await flush();
        expect(fn()).toHaveBeenCalledTimes(1);
    });

    it('single-shot mode (autoPoll false) also waits for visibility and never polls', async () => {
        const dom = installDom(false);
        const hook = await load();
        render(() => hook(true, true, false));
        await flush();
        expect(fn()).not.toHaveBeenCalled();
        dom.set(true);
        await flush();
        expect(fn()).toHaveBeenCalledTimes(1);
        vi.advanceTimersByTime(120_000);
        await flush();
        expect(fn()).toHaveBeenCalledTimes(1);
    });

    it('polls while visible and skips hidden ticks', async () => {
        const dom = installDom(true);
        const hook = await load();
        render(() => hook(true, true, true));
        await flush();
        expect(fn()).toHaveBeenCalledTimes(1);
        vi.advanceTimersByTime(30_000);
        await flush();
        expect(fn()).toHaveBeenCalledTimes(2);
        dom.set(false);
        vi.advanceTimersByTime(90_000);
        await flush();
        expect(fn()).toHaveBeenCalledTimes(2);
        dom.set(true);
        await flush();
        expect(fn()).toHaveBeenCalledTimes(3);
    });

    it('does nothing while the consumer is not ready', async () => {
        installDom(true);
        const hook = await load();
        render(() => hook(true, false, true));
        await flush();
        expect(fn()).not.toHaveBeenCalled();
    });
});
