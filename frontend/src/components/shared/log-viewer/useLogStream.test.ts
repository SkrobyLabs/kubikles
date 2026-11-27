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

const emit = (name: string, event: any) => mocks.handlers.get(name)?.forEach(fn => fn(event));
const ts = (n: number) => `2024-05-01T10:00:${String(n).padStart(2, '0')}.000000000Z`;
const line = (n: number, text: string) => `${ts(n)} ${text}`;

function mount(overrides: Record<string, any> = {}) {
    const props = {
        namespace: 'default', pod: 'web-1', container: 'main', containers: ['main'], siblingPods: [], podContainerMap: {},
        showPrevious: false, sinceTime: '', viewMode: 'end', initialPosition: 'end', isStale: false, currentContext: 'ctx',
        ...overrides,
    };
    let result: any;
    const render = () => {
        for (let i = 0; i < 20; i++) {
            runner.index = 0; runner.effects = [];
            result = useLogStream(props as any);
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
    return { flush, render, get: () => result };
}

beforeEach(() => {
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    mocks.handlers.clear();
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

describe('useLogStream initial load', () => {
    it('parses timestamped lines into entries tagged with their source', async () => {
        const view = mount();
        const result = await view.flush();
        expect(mocks.App.GetPodLogs).toHaveBeenCalledWith('default', 'web-1', 'main', true, false, '');
        expect(result.logs).toEqual([
            { timestamp: ts(1), content: 'one', source: 'initial' },
            { timestamp: ts(2), content: 'two', source: 'initial' },
            { timestamp: ts(3), content: 'three', source: 'initial' },
        ]);
        expect(result.loading).toBe(false);
        expect(result.getFirstTimestamp()).toBe(ts(1));
        expect(result.getLastTimestamp()).toBe(ts(3));
        expect(result.hasMoreBefore).toBe(true);
        expect(result.hasMoreAfter).toBe(false);
    });

    it('keeps lines without a timestamp and drops blank lines', async () => {
        mocks.App.GetPodLogs = vi.fn(async () => `${line(1, 'a')}\n\n   \nplain text\n${line(2, 'b')}`);
        const result = await mount().flush();
        expect(result.logs.map((e: any) => [e.timestamp, e.content])).toEqual([
            [ts(1), 'a'], ['', 'plain text'], [ts(2), 'b'],
        ]);
        expect(result.getLastTimestamp()).toBe(ts(2));
    });

    it('reports fetch failures without crashing and leaves the log list empty', async () => {
        mocks.App.GetPodLogs = vi.fn(async () => { throw new Error('boom'); });
        const result = await mount().flush();
        expect(result.logs).toEqual([]);
        expect(result.fetchError).toContain('boom');
        expect(result.loading).toBe(false);
    });

    it('does not fetch without a namespace or pod', async () => {
        const view = mount({ namespace: '' });
        await view.flush();
        expect(mocks.App.GetPodLogs).not.toHaveBeenCalled();
    });
});

describe('useLogStream processLine (single stream events)', () => {
    it('appends a newer line with the stream source', async () => {
        const view = await streaming();
        emit('log-stream', { streamId: 'stream-1', line: line(4, 'four') });
        expect((await view.flush()).logs.at(-1)).toEqual({ timestamp: ts(4), content: 'four', source: 'stream' });
        expect(view.get().logs).toHaveLength(4);
    });

    it('drops lines at or before the last known timestamp', async () => {
        const view = await streaming();
        emit('log-stream', { streamId: 'stream-1', line: line(3, 'duplicate of last') });
        emit('log-stream', { streamId: 'stream-1', line: line(1, 'older') });
        await view.flush();
        expect(view.get().logs).toHaveLength(3);
    });

    it('keeps untimestamped lines even when older lines would be dropped', async () => {
        const view = await streaming();
        emit('log-stream', { streamId: 'stream-1', line: 'no timestamp here' });
        await view.flush();
        expect(view.get().logs.at(-1)).toEqual({ timestamp: '', content: 'no timestamp here', source: 'stream' });
    });

    it('uses the last timestamped entry (skipping trailing untimestamped ones) as the cursor', async () => {
        const view = await streaming();
        emit('log-stream', { streamId: 'stream-1', line: 'continuation' });
        await view.flush();
        emit('log-stream', { streamId: 'stream-1', line: line(3, 'still a duplicate') });
        emit('log-stream', { streamId: 'stream-1', line: line(5, 'fresh') });
        await view.flush();
        expect(view.get().logs.map((e: any) => e.content)).toEqual(['one', 'two', 'three', 'continuation', 'fresh']);
    });

    it('accepts everything when the list is still empty', async () => {
        mocks.App.GetPodLogs = vi.fn(async () => '');
        const view = await streaming();
        emit('log-stream', { streamId: 'stream-1', line: line(1, 'first') });
        await view.flush();
        expect(view.get().logs.map((e: any) => e.content)).toEqual(['first']);
    });

    it('ignores events for other streams or when no stream is active', async () => {
        const view = mount();
        await view.flush();
        emit('log-stream', { streamId: 'stream-1', line: line(9, 'before start') });
        await view.get().startStreaming();
        emit('log-stream', { streamId: 'other', line: line(9, 'other stream') });
        await view.flush();
        expect(view.get().logs).toHaveLength(3);
    });

    it('marks the stream disconnected on done and on error', async () => {
        const view = await streaming();
        emit('log-stream', { streamId: 'stream-1', done: true });
        await view.flush();
        expect(view.get().streamDisconnected).toBe(true);
        expect(view.get().disconnectReason).toBe('Pod terminated or container restarted');
        expect(view.get().isStreaming()).toBe(false);

        const second = await streaming();
        vi.spyOn(console, 'error').mockImplementation(() => {});
        emit('log-stream', { streamId: 'stream-1', error: 'connection reset' });
        await second.flush();
        expect(second.get().streamDisconnected).toBe(true);
        expect(second.get().disconnectReason).toBe('connection reset');
    });
});

describe('useLogStream processBatch (coalesced events)', () => {
    it('appends a batch in order with a single state update', async () => {
        const view = await streaming();
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(4, 'four'), line(5, 'five'), line(6, 'six')] });
        await view.flush();
        expect(view.get().logs.map((e: any) => e.content)).toEqual(['one', 'two', 'three', 'four', 'five', 'six']);
    });

    it('filters against the previous last timestamp only, not within the batch', async () => {
        const view = await streaming();
        emit('log-stream-batch', { streamId: 'stream-1', lines: [
            line(2, 'old'), line(4, 'new'), line(4, 'same-second'), line(3, 'out of order but newer than last'),
        ] });
        await view.flush();
        // Entries at or before ts(3) are dropped as stale. Entries within the batch are never compared with each other.
        expect(view.get().logs.map((e: any) => e.content)).toEqual(['one', 'two', 'three', 'new', 'same-second']);
    });

    it('parses multi-line payload strings inside a batch', async () => {
        const view = await streaming();
        emit('log-stream-batch', { streamId: 'stream-1', lines: [`${line(4, 'a')}\n${line(5, 'b')}`, 'raw'] });
        await view.flush();
        expect(view.get().logs.slice(3).map((e: any) => e.content)).toEqual(['a', 'b', 'raw']);
    });

    it('leaves state untouched for empty or foreign batches and keeps array identity when all are stale', async () => {
        const view = await streaming();
        const before = view.get().logs;
        emit('log-stream-batch', { streamId: 'stream-1', lines: [] });
        emit('log-stream-batch', { streamId: 'other', lines: [line(9, 'x')] });
        emit('log-stream-batch', { streamId: 'stream-1', lines: [line(1, 'stale')] });
        await view.flush();
        expect(view.get().logs).toBe(before);
    });

    it('handles a large burst: 5000 fresh lines after 3 existing, and a stale replay of the same burst', async () => {
        const view = await streaming();
        const burst = Array.from({ length: 5000 }, (_, i) =>
            `2024-05-01T11:${String(Math.floor(i / 60)).padStart(2, '0')}:${String(i % 60).padStart(2, '0')}.000000000Z msg-${i}`);
        emit('log-stream-batch', { streamId: 'stream-1', lines: burst });
        await view.flush();
        expect(view.get().logs).toHaveLength(5003);
        expect(view.get().logs[3].content).toBe('msg-0');
        expect(view.get().logs.at(-1).content).toBe('msg-4999');
        // Replaying the same burst is entirely at-or-before the last timestamp, except the strictly newer ones (none).
        emit('log-stream-batch', { streamId: 'stream-1', lines: burst });
        await view.flush();
        expect(view.get().logs).toHaveLength(5003);
    });
});

describe('useLogStream polling', () => {
    it('requests lines after the last timestamp and appends only unseen entries', async () => {
        const view = mount();
        await view.flush();
        mocks.App.GetPodLogsAfter = vi.fn(async () => ({
            logs: [line(3, 'three'), line(4, 'four')].join('\n'), hasMore: false,
        }));
        await view.get().pollNewerLogs();
        await view.flush();
        expect(mocks.App.GetPodLogsAfter).toHaveBeenCalledWith('default', 'web-1', 'main', true, false, ts(3), 200);
        expect(view.get().logs.map((e: any) => e.content)).toEqual(['one', 'two', 'three', 'four']);
        expect(view.get().fetchError).toBeNull();
    });
});
