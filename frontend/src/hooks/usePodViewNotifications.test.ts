import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Pod notifications for a list served by a Go view: the view reports transitions (restarts of
// visible pods, deletions of visible pods, new pods) and usePodViewNotifications turns them into
// the same messages the legacy snapshot diff produces. The parity test drives both with the same
// pod history and compares what the user would see.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void> }));
const mocks = vi.hoisted(() => ({ addNotification: vi.fn() }));

vi.mock('react', () => {
    const memo = (factory: () => any, deps: any[]) => {
        const index = runner.index++;
        const old = runner.slots[index];
        if (!old || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) runner.slots[index] = { value: factory(), deps };
        return runner.slots[index].value;
    };
    return {
        useRef: (initial: any) => memo(() => ({ current: initial }), []),
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
vi.mock('../context', () => ({ useNotification: () => ({ addNotification: mocks.addNotification }) }));
vi.mock('../components/shared/NotificationSettingsMenu', () => ({
    NOTIFICATION_SOUNDS: { alert: { notes: [] }, warning: { notes: [] }, info: { notes: [] }, none: { notes: [] } },
}));

import { eventsFromTransitions, eventsToNotifications, usePodNotifications, usePodViewNotifications } from './usePodNotifications';
import type { PodTransition } from '../lib/views/protocol';

type Pod = any;
const pod = (name: string, restarts: Array<[string, number]> = [['main', 0]], owner?: string, ns = 'default'): Pod => ({
    metadata: { name, namespace: ns, uid: `uid-${ns}-${name}`, ownerReferences: owner ? [{ kind: 'ReplicaSet', name: owner }] : undefined },
    status: { containerStatuses: restarts.map(([n, c]) => ({ name: n, restartCount: c })) },
});
const total = (p: Pod) => (p.status.containerStatuses as any[]).reduce((n, c) => n + c.restartCount, 0);

function mountLegacy(initial: Pod[]) {
    const state = { pods: initial, filtered: null as Set<string> | null };
    const slots: any[] = [];
    const render = () => {
        runner.slots = slots; runner.index = 0; runner.effects = [];
        usePodNotifications(state.pods, true, state.filtered, { soundKey: 'none' });
        runner.effects.forEach(effect => effect());
    };
    render();
    return { update: (pods: Pod[], filtered: Set<string> | null) => { state.pods = pods; state.filtered = filtered; render(); } };
}

function mountView() {
    let listener: ((events: PodTransition[]) => void) | null = null;
    const view = { onTransitions: (cb: (events: PodTransition[]) => void) => { listener = cb; return () => { listener = null; }; } };
    const state = { enabled: true };
    const slots: any[] = [];
    const render = () => {
        runner.slots = slots; runner.index = 0; runner.effects = [];
        usePodViewNotifications(view, state.enabled, { soundKey: 'none' });
        runner.effects.forEach(effect => effect());
    };
    render();
    return {
        emit: (events: PodTransition[]) => listener?.(events),
        setEnabled: (enabled: boolean) => { state.enabled = enabled; render(); },
        subscribed: () => listener !== null,
    };
}

const added = () => mocks.addNotification.mock.calls.map(call => call[0]);

beforeEach(() => {
    runner.slots = []; runner.index = 0; runner.effects = [];
    mocks.addNotification.mockClear();
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-05-01T10:00:00Z'));
});
afterEach(() => vi.useRealTimers());

describe('usePodViewNotifications', () => {
    it('turns a restart transition into the legacy message', () => {
        const m = mountView();
        m.emit([{ kind: 'restart', uid: 'u', namespace: 'prod', name: 'web-1', total: 3, containers: [{ name: 'app', count: 2 }, { name: 'side', count: 1 }] }]);
        vi.advanceTimersByTime(1);
        expect(added()).toEqual([{ type: 'warning', title: 'Pod Restart', message: 'web-1 in prod restarted — app (2), side (1)' }]);
    });

    it('falls back to the total when no container is named', () => {
        const m = mountView();
        m.emit([{ kind: 'restart', uid: 'u', namespace: 'prod', name: 'web-1', total: 4 }]);
        vi.advanceTimersByTime(1);
        expect(added()[0].message).toBe('web-1 in prod restarted — total: 4');
    });

    it('reports a redeploy when a pod of the same owner appears after a deletion, in either order within a notice', () => {
        const m = mountView();
        m.emit([
            { kind: 'added', uid: 'n', namespace: 'd', name: 'web-2', owner: 'ReplicaSet/web' },
            { kind: 'deleted', uid: 'o', namespace: 'd', name: 'web-1', owner: 'ReplicaSet/web' },
        ]);
        vi.advanceTimersByTime(1);
        expect(added()).toEqual([{ type: 'info', title: 'Pod Redeployed', message: 'web-2 replaced web-1 (ReplicaSet: web)' }]);
    });

    it('remembers a deletion across notices for the redeploy window only', () => {
        const m = mountView();
        m.emit([{ kind: 'deleted', uid: 'o', namespace: 'd', name: 'web-1', owner: 'ReplicaSet/web' }]);
        vi.advanceTimersByTime(31000);
        m.emit([{ kind: 'added', uid: 'n', namespace: 'd', name: 'web-2', owner: 'ReplicaSet/web' }]);
        vi.advanceTimersByTime(1);
        expect(added()).toEqual([]);
    });

    it('ignores new pods without an owner and stays silent while disabled', () => {
        const m = mountView();
        m.emit([{ kind: 'deleted', uid: 'o', namespace: 'd', name: 'a', owner: 'ReplicaSet/web' }, { kind: 'added', uid: 'n', namespace: 'd', name: 'b' }]);
        vi.advanceTimersByTime(1);
        expect(added()).toEqual([]);
        m.setEnabled(false);
        expect(m.subscribed()).toBe(false);
        m.emit([{ kind: 'restart', uid: 'u', namespace: 'd', name: 'x', total: 1 }]);
        vi.advanceTimersByTime(1);
        expect(added()).toEqual([]);
    });

    it('forgets deletions when disabled, like the legacy hook dropping its snapshot', () => {
        const m = mountView();
        m.emit([{ kind: 'deleted', uid: 'o', namespace: 'd', name: 'web-1', owner: 'ReplicaSet/web' }]);
        m.setEnabled(false);
        m.setEnabled(true);
        m.emit([{ kind: 'added', uid: 'n', namespace: 'd', name: 'web-2', owner: 'ReplicaSet/web' }]);
        vi.advanceTimersByTime(1);
        expect(added()).toEqual([]);
    });
});

describe('eventsToNotifications', () => {
    it('runs restarts, then deletions, then redeploys whatever the event order', () => {
        const events = eventsFromTransitions([
            { kind: 'added', uid: 'n', namespace: 'd', name: 'web-2', owner: 'ReplicaSet/web' },
            { kind: 'restart', uid: 'u', namespace: 'd', name: 'db-0', total: 1, containers: [{ name: 'db', count: 1 }] },
            { kind: 'deleted', uid: 'o', namespace: 'd', name: 'web-1', owner: 'ReplicaSet/web' },
        ]);
        expect(eventsToNotifications(events, new Map()).map(p => p.notification.title)).toEqual(['Pod Restart', 'Pod Redeployed']);
    });
});

// What the Go view reports for a step of a pod history, written out independently here: restarts
// and deletions of pods that the filtered list held before the step, every new pod.
function referenceTransitions(prev: Pod[], next: Pod[], visible: Set<string> | null): PodTransition[] {
    const isVisible = (uid: string) => !visible || visible.has(uid);
    const prevBy = new Map<string, Pod>(prev.map(p => [p.metadata.uid, p]));
    const nextBy = new Map<string, Pod>(next.map(p => [p.metadata.uid, p]));
    const out: PodTransition[] = [];
    const owner = (p: Pod) => (p.metadata.ownerReferences?.[0] ? `${p.metadata.ownerReferences[0].kind}/${p.metadata.ownerReferences[0].name}` : undefined);
    for (const p of next) {
        const old = prevBy.get(p.metadata.uid);
        if (old && total(p) > total(old) && isVisible(p.metadata.uid)) {
            const before = new Map<string, number>(old.status.containerStatuses.map((c: any) => [c.name, c.restartCount]));
            out.push({
                kind: 'restart', uid: p.metadata.uid, namespace: p.metadata.namespace, name: p.metadata.name, owner: owner(p), total: total(p),
                containers: p.status.containerStatuses.filter((c: any) => c.restartCount > (before.get(c.name) ?? 0)).map((c: any) => ({ name: c.name, count: c.restartCount })),
            });
        }
    }
    for (const p of prev) {
        if (!nextBy.has(p.metadata.uid) && isVisible(p.metadata.uid)) {
            out.push({ kind: 'deleted', uid: p.metadata.uid, namespace: p.metadata.namespace, name: p.metadata.name, owner: owner(p) });
        }
    }
    for (const p of next) {
        if (!prevBy.has(p.metadata.uid)) out.push({ kind: 'added', uid: p.metadata.uid, namespace: p.metadata.namespace, name: p.metadata.name, owner: owner(p) });
    }
    return out;
}

describe('view and legacy paths give the same notifications', () => {
    it('over a seeded history of restarts, deletions, replacements and filters', () => {
        let seed = 7;
        const rnd = () => { seed = (seed * 1664525 + 1013904223) >>> 0; return seed / 4294967296; };
        let n = 0;
        const fresh = (owner?: string): Pod => pod(`p${++n}`, [['a', 0], ['b', 0]], owner, rnd() > 0.5 ? 'x' : 'y');

        let pods: Pod[] = Array.from({ length: 12 }, (_, i) => fresh(i % 3 === 0 ? `rs${i % 2}` : undefined));
        const legacy = mountLegacy(pods);
        const view = mountView();
        let filtered: Set<string> | null = null;

        for (let step = 0; step < 40; step++) {
            const next = pods.map(p => structuredClone(p));
            const ops = 1 + Math.floor(rnd() * 4);
            for (let k = 0; k < ops && next.length > 0; k++) {
                const i = Math.floor(rnd() * next.length);
                const r = rnd();
                if (r < 0.4) next[i].status.containerStatuses[Math.floor(rnd() * 2)].restartCount += 1;
                else if (r < 0.65) next.splice(i, 1);
                else next.push(fresh(`rs${Math.floor(rnd() * 2)}`));
            }
            // The filter the list shows before this step (the legacy hook sees it one render late).
            const before: Set<string> | null = rnd() > 0.5 ? null : new Set(pods.filter(() => rnd() > 0.3).map(p => p.metadata.uid));
            filtered = before;

            mocks.addNotification.mockClear();
            legacy.update(next, filtered);
            vi.advanceTimersByTime(1);
            const fromLegacy = added();

            mocks.addNotification.mockClear();
            view.emit(referenceTransitions(pods, next, filtered));
            vi.advanceTimersByTime(1);
            const fromView = added();

            expect(fromView, `step ${step}`).toEqual(fromLegacy);
            pods = next;
        }
    });
});
