import { beforeEach, describe, expect, it, vi } from 'vitest';

// Dependency-aware hook runner: runs the production hook with real effects and state,
// so the stream event handlers registered in effects can be driven exactly as Wails would.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
const mocks = vi.hoisted(() => ({
    handlers: new Map<string, Set<(event: any) => void>>(),
    App: {} as Record<string, any>,
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
                    const value = typeof next === 'function' ? next(slot.value) : next;
                    if (!Object.is(value, slot.value)) { slot.value = value; runner.dirty = true; }
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

vi.mock('wailsjs/go/main/App', () => {
    const names = [
        'GetPodLogs', 'GetAllPodLogs', 'GetPodLogsFromStart', 'GetPodLogsBefore', 'GetPodLogsAfter',
        'StartLogStream', 'StopLogStream',
        'GetAllContainersLogs', 'GetAllContainersLogsAll', 'GetAllContainersLogsFromStart',
        'GetAllContainersLogsBefore', 'GetAllContainersLogsAfter', 'StartAllContainersLogStream',
        'GetAllPodsLogs', 'GetAllPodsLogsAll', 'GetAllPodsLogsFromStart',
        'GetAllPodsLogsBefore', 'GetAllPodsLogsAfter', 'StartAllPodsLogStream',
    ];
    // Delegate through mocks.App so each test can install its own implementations.
    return Object.fromEntries(names.map(name => [name, (...args: any[]) => mocks.App[name]?.(...args)]));
});
vi.mock('wailsjs/runtime/runtime', () => ({
    EventsOn: (name: string, fn: any) => {
        if (!mocks.handlers.has(name)) mocks.handlers.set(name, new Set());
        mocks.handlers.get(name)!.add(fn);
        return () => mocks.handlers.get(name)!.delete(fn);
    },
}));

import { useLogStream } from './useLogStream';
import { LIVE_LOG_CAP } from './logBuffer';

const emit = (name: string, event: any) => mocks.handlers.get(name)?.forEach(fn => fn(event));
const ts = (n: number) => new Date(Date.UTC(2024, 4, 1) + n * 1000).toISOString().replace('.000Z', '.000000000Z');
const line = (n: number, text: string) => `${ts(n)} ${text}`;

function mount(overrides: Record<string, any> = {}) {
    const props: any = {
        namespace: 'default', pod: 'web-1', container: 'main', containers: ['main'], siblingPods: [], podContainerMap: {},
        showPrevious: false, sinceTime: '', viewMode: 'end', initialPosition: 'end', isStale: false, currentContext: 'ctx',
        ...overrides,
    };
    let result: any;
    let renders = 0;
    const render = () => {
        for (let i = 0; i < 20; i++) {
            runner.index = 0; runner.effects = [];
            result = useLogStream(props);
            renders++;
            runner.dirty = false;
            runner.effects.forEach(effect => effect());
            if (!runner.dirty) break;
        }
        return result;
    };
    const flush = async () => {
        for (let i = 0; i < 30; i++) await Promise.resolve();
        return render();
    };
    render();
    return { flush, render, get: () => result, props, renders: () => renders };
}

let frames: Array<() => void> = [];
const runFrames = () => { const pending = frames; frames = []; pending.forEach(fn => fn()); };

beforeEach(() => {
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    mocks.handlers.clear();
    frames = [];
    delete (globalThis as any).requestAnimationFrame;
    delete (globalThis as any).cancelAnimationFrame;
    mocks.App = {
        GetPodLogs: vi.fn(async () => [line(1, 'one'), line(2, 'two'), line(3, 'three')].join('\n')),
        StartLogStream: vi.fn(async () => 'stream-1'),
        StopLogStream: vi.fn(),
    };
});

async function streaming(overrides: Record<string, any> = {}) {
    const view = mount(overrides);
    await view.flush();
    await view.get().startStreaming();
    return view;
}

function useFrames() {
    let nextId = 1;
    (globalThis as any).requestAnimationFrame = (fn: () => void) => { frames.push(fn); return nextId++; };
    (globalThis as any).cancelAnimationFrame = () => {};
}

describe('useLogStream bounded buffer', () => {
    it('trims the head past the live cap, keeps ids stable and reports the trim', async () => {
        const view = await streaming();
        const before = view.get().firstItemIndex;
        const burst = Array.from({ length: LIVE_LOG_CAP + 10_000 }, (_, i) => line(100 + i, `msg-${i}`));
        emit('log-stream-batch', { streamId: 'stream-1', lines: burst });
        const result = await view.flush();
        expect(result.logCount).toBe(LIVE_LOG_CAP);
        expect(result.logs.length).toBe(LIVE_LOG_CAP);
        const dropped = 3 + 10_000;
        expect(result.trimmedCount).toBe(dropped);
        expect(result.firstItemIndex).toBe(before + dropped);
        // The line with id firstItemIndex is the oldest retained one, and the newest is intact.
        expect(result.getEntryById(result.firstItemIndex).content).toBe('msg-10000');
        expect(result.logs.at(-1).content).toBe(`msg-${LIVE_LOG_CAP + 9_999}`);
        // Trimmed lines can be re-fetched by scrolling up.
        expect(result.hasMoreBefore).toBe(true);
    });

    it('keeps the last timestamp cursor across trimming so replays are still dropped', async () => {
        const view = await streaming();
        emit('log-stream-batch', { streamId: 'stream-1', lines: Array.from({ length: LIVE_LOG_CAP + 6000 }, (_, i) => line(100 + i, `m${i}`)) });
        await view.flush();
        const count = view.get().logCount;
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(100 + LIVE_LOG_CAP + 5999, 'replay')] });
        expect((await view.flush()).logCount).toBe(count);
    });
});

describe('useLogStream unavailable older lines', () => {
    it('stops advertising dropped lines once they cannot be fetched again', async () => {
        const view = await streaming();
        emit('log-stream-batch', { streamId: 'stream-1', lines: Array.from({ length: LIVE_LOG_CAP + 6000 }, (_, i) => line(100 + i, `m${i}`)) });
        await view.flush();
        expect(view.get().hasMoreBefore).toBe(true);
        mocks.App.GetPodLogsBefore = vi.fn(async () => ({ logs: '', hasMore: false }));
        await view.get().loadOlderLogs();
        await view.flush();
        expect(view.get().hasMoreBefore).toBe(false);
        expect(view.get().trimmedCount).toBe(0);
        // A later streamed publish must not flip it back on.
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(100 + LIVE_LOG_CAP + 6000, 'next')] });
        const result = await view.flush();
        expect(result.hasMoreBefore).toBe(false);
    });
});

describe('useLogStream publish coalescing', () => {
    it('publishes several batches that arrive in one frame as a single snapshot', async () => {
        const view = await streaming();
        useFrames();
        const rendersBefore = view.renders();
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(10, 'a')] });
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(11, 'b')] });
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(12, 'c')] });
        view.render();
        // Nothing published yet: still the three initial lines.
        expect(view.get().logCount).toBe(3);
        expect(frames).toHaveLength(1);
        runFrames();
        const result = view.render();
        expect(result.logCount).toBe(6);
        expect(result.logs.slice(3).map((x: any) => x.content)).toEqual(['a', 'b', 'c']);
        expect(view.renders() - rendersBefore).toBeLessThanOrEqual(3);
    });
});

describe('useLogStream hidden tabs', () => {
    it('keeps receiving lines while inactive without publishing, then catches up on activation', async () => {
        const view = await streaming();
        useFrames();
        view.props.isActive = false;
        view.render();
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(10, 'hidden-1'), line(11, 'hidden-2')] });
        view.render();
        expect(frames).toHaveLength(0);
        expect(view.get().logCount).toBe(3);
        // Direct reads still see the buffered lines (download/copy use these).
        expect(view.get().getEntries().map((x: any) => x.content).slice(3)).toEqual(['hidden-1', 'hidden-2']);

        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(12, 'hidden-3')] });
        view.props.isActive = true;
        view.render();
        expect(view.get().logCount).toBe(6);
    });
});
