import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// Drives the real hook (effects, refs) with a minimal runner so the snapshot diffing
// can be characterized: what triggers a notification, and what never does.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void> }));
const mocks = vi.hoisted(() => ({ addNotification: vi.fn(), oscillators: 0, resumed: 0 }));

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
    NOTIFICATION_SOUNDS: {
        alert: { notes: [{ freq: 440, start: 0 }, { freq: 660, start: 0.1 }] },
        warning: { notes: [{ freq: 300, start: 0 }] },
        info: { notes: [{ freq: 500, start: 0 }] },
        none: { notes: [] },
    },
}));

import { usePodNotifications } from './usePodNotifications';

type Pod = any;
const pod = (name: string, restarts = 0, extra: { owner?: string; ns?: string; uid?: string; containers?: Array<[string, number]> } = {}): Pod => ({
    metadata: {
        name, namespace: extra.ns ?? 'default', uid: extra.uid ?? `uid-${name}`,
        ownerReferences: extra.owner ? [{ kind: 'ReplicaSet', name: extra.owner }] : undefined,
    },
    status: {
        containerStatuses: (extra.containers ?? [['main', restarts]]).map(([n, c]) => ({ name: n, restartCount: c })),
    },
});

function mount(initial: { pods: Pod[] | null; enabled?: boolean; filtered?: Set<string> | null; settings?: any }) {
    const state = { pods: initial.pods, enabled: initial.enabled ?? true, filtered: initial.filtered ?? null, settings: initial.settings ?? {} };
    const render = () => {
        runner.index = 0; runner.effects = [];
        usePodNotifications(state.pods, state.enabled, state.filtered, state.settings);
        runner.effects.forEach(effect => effect());
    };
    render();
    return {
        update: (patch: Partial<typeof state>) => { Object.assign(state, patch); render(); },
        // Notifications are delivered from a zero-delay timer, outside the commit phase.
        flush: () => vi.advanceTimersByTime(1),
    };
}
const added = () => mocks.addNotification.mock.calls.map(call => call[0]);

beforeEach(() => {
    runner.slots = []; runner.index = 0; runner.effects = [];
    mocks.addNotification.mockClear(); mocks.oscillators = 0; mocks.resumed = 0;
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2024-05-01T10:00:00Z'));
    const gain = () => ({ connect: vi.fn(), gain: { setValueAtTime: vi.fn(), linearRampToValueAtTime: vi.fn(), exponentialRampToValueAtTime: vi.fn() } });
    (globalThis as any).window = {
        AudioContext: class {
            state = 'running'; currentTime = 0; destination = {};
            resume() { mocks.resumed++; }
            createOscillator() { mocks.oscillators++; return { connect: vi.fn(), frequency: { value: 0 }, start: vi.fn(), stop: vi.fn(), type: '' }; }
            createGain() { return gain(); }
        },
    };
});
afterEach(() => { vi.useRealTimers(); });

describe('usePodNotifications restarts', () => {
    it('does not notify on the first snapshot, even for pods that already restarted', () => {
        const view = mount({ pods: [pod('a', 5)] });
        view.flush();
        expect(added()).toEqual([]);
    });

    it('notifies when a container restart count increases and names the container', () => {
        const view = mount({ pods: [pod('a', 0), pod('b', 2)] });
        view.update({ pods: [pod('a', 1), pod('b', 2)] });
        view.flush();
        expect(added()).toEqual([{ type: 'warning', title: 'Pod Restart', message: 'a in default restarted — main (1)' }]);
    });

    it('lists only the containers whose count rose', () => {
        const view = mount({ pods: [pod('a', 0, { containers: [['app', 1], ['side', 4]] })] });
        view.update({ pods: [pod('a', 0, { containers: [['app', 2], ['side', 4]] })] });
        view.flush();
        expect(added()[0].message).toBe('a in default restarted — app (2)');
    });

    it('does not notify when nothing changed, when counts fall, or for a brand-new pod with restarts', () => {
        const view = mount({ pods: [pod('a', 3)] });
        view.update({ pods: [pod('a', 3)] });
        view.update({ pods: [pod('a', 1)] });
        view.update({ pods: [pod('a', 1), pod('fresh', 9)] });
        view.flush();
        expect(added()).toEqual([]);
    });

    it('emits one notification per restarted pod in a single update', () => {
        const view = mount({ pods: [pod('a', 0), pod('b', 0), pod('c', 0)] });
        view.update({ pods: [pod('a', 1), pod('b', 0), pod('c', 4)] });
        view.flush();
        expect(added().map(n => n.message)).toEqual([
            'a in default restarted — main (1)', 'c in default restarted — main (4)',
        ]);
    });

    it('ignores pods without a uid', () => {
        const noUid = { metadata: { name: 'x' }, status: { containerStatuses: [{ name: 'm', restartCount: 0 }] } };
        const view = mount({ pods: [noUid, pod('a', 0)] });
        view.update({ pods: [{ ...noUid, status: { containerStatuses: [{ name: 'm', restartCount: 5 }] } }, pod('a', 0)] });
        view.flush();
        expect(added()).toEqual([]);
    });
});

describe('usePodNotifications gating', () => {
    it('fires nothing while disabled and does not replay missed restarts after enabling', () => {
        const view = mount({ pods: [pod('a', 0)], enabled: false });
        view.update({ pods: [pod('a', 2)] });
        view.flush();
        expect(added()).toEqual([]);
        expect(mocks.oscillators).toBe(0);
        view.update({ enabled: true });
        view.flush();
        expect(added()).toEqual([]);
        view.update({ pods: [pod('a', 3)] });
        view.flush();
        expect(added()).toHaveLength(1);
    });

    it('drops its snapshot while disabled so re-enabling only reseeds (no burst, no stale redeploy)', () => {
        const view = mount({ pods: [pod('web-old', 0, { owner: 'web' })], enabled: true });
        view.update({ enabled: false });
        // A restart and a replacement happen while disabled
        view.update({ pods: [pod('web-new', 4, { owner: 'web' })] });
        view.update({ enabled: true });
        view.flush();
        expect(added()).toEqual([]);
        view.update({ pods: [pod('web-new', 5, { owner: 'web' })] });
        view.flush();
        expect(added().map(n => n.title)).toEqual(['Pod Restart']);
    });

    it('does nothing for null pods and treats an empty list as a reset, not as mass deletion', () => {
        const view = mount({ pods: null });
        view.update({ pods: [pod('a', 0, { owner: 'rs' })] });
        view.update({ pods: [] });
        view.update({ pods: [pod('b', 0, { owner: 'rs' })] });
        view.flush();
        expect(added()).toEqual([]);
    });

    it('restricts restart notifications to the filtered uid set', () => {
        const filtered = new Set(['uid-a']);
        const view = mount({ pods: [pod('a', 0), pod('b', 0)], filtered });
        view.update({ pods: [pod('a', 1), pod('b', 1)] });
        view.flush();
        expect(added().map(n => n.message)).toEqual(['a in default restarted — main (1)']);
    });

    it('treats a null filter as "everything visible"', () => {
        const view = mount({ pods: [pod('a', 0), pod('b', 0)], filtered: null });
        view.update({ pods: [pod('a', 1), pod('b', 1)] });
        view.flush();
        expect(added()).toHaveLength(2);
    });
});

describe('usePodNotifications redeploys', () => {
    it('reports a replacement pod from the same owner shortly after a deletion', () => {
        const view = mount({ pods: [pod('web-old', 0, { owner: 'web' })] });
        view.update({ pods: [] });
        view.update({ pods: [pod('web-old', 0, { owner: 'web' })] });
        view.update({ pods: [pod('web-new', 0, { owner: 'web' })] });
        view.flush();
        expect(added()).toEqual([{
            type: 'info', title: 'Pod Redeployed', message: 'web-new replaced web-old (ReplicaSet: web)',
        }]);
    });

    it('detects a swap within one snapshot', () => {
        const view = mount({ pods: [pod('web-old', 0, { owner: 'web' }), pod('keep', 0)] });
        view.update({ pods: [pod('web-new', 0, { owner: 'web' }), pod('keep', 0)] });
        view.flush();
        expect(added().map(n => n.title)).toEqual(['Pod Redeployed']);
    });

    it('does not report a replacement after the 30 second window has passed', () => {
        const view = mount({ pods: [pod('web-old', 0, { owner: 'web' }), pod('keep', 0)] });
        view.update({ pods: [pod('keep', 0)] });
        vi.setSystemTime(new Date('2024-05-01T10:00:31Z'));
        view.update({ pods: [pod('keep', 0), pod('web-new', 0, { owner: 'web' })] });
        view.flush();
        expect(added()).toEqual([]);
    });

    it('does not report new pods that have no owner or a different owner', () => {
        const view = mount({ pods: [pod('web-old', 0, { owner: 'web' }), pod('keep', 0)] });
        view.update({ pods: [pod('keep', 0), pod('loner', 0), pod('other', 0, { owner: 'other' })] });
        view.flush();
        expect(added()).toEqual([]);
    });

    it('does not report a deletion filtered out of view', () => {
        const view = mount({ pods: [pod('web-old', 0, { owner: 'web' }), pod('keep', 0)], filtered: new Set(['uid-keep']) });
        view.update({ pods: [pod('keep', 0), pod('web-new', 0, { owner: 'web' })] });
        view.flush();
        expect(added()).toEqual([]);
    });

    it('reports each deletion only once', () => {
        const view = mount({ pods: [pod('web-old', 0, { owner: 'web' }), pod('keep', 0)] });
        view.update({ pods: [pod('keep', 0), pod('web-new-1', 0, { owner: 'web' })] });
        view.update({ pods: [pod('keep', 0), pod('web-new-1', 0, { owner: 'web' }), pod('web-new-2', 0, { owner: 'web' })] });
        view.flush();
        expect(added().map(n => n.message)).toEqual(['web-new-1 replaced web-old (ReplicaSet: web)']);
    });
});

describe('usePodNotifications sound and throttling', () => {
    const restartOnce = (settings: any) => {
        const view = mount({ pods: [pod('a', 0)], settings });
        return view;
    };

    it('delivers notifications from a deferred callback, not synchronously', () => {
        const view = restartOnce({});
        view.update({ pods: [pod('a', 1)] });
        expect(added()).toEqual([]);
        view.flush();
        expect(added()).toHaveLength(1);
    });

    it('plays the configured sound once per batch', () => {
        const view = mount({ pods: [pod('a', 0), pod('b', 0)], settings: { soundKey: 'alert' } });
        view.update({ pods: [pod('a', 1), pod('b', 1)] });
        view.flush();
        expect(added()).toHaveLength(2);
        expect(mocks.oscillators).toBe(2);
    });

    it('stays silent for the "none" sound but still notifies', () => {
        const view = restartOnce({ soundKey: 'none' });
        view.update({ pods: [pod('a', 1)] });
        view.flush();
        expect(added()).toHaveLength(1);
        expect(mocks.oscillators).toBe(0);
    });

    it('still lists notifications but mutes the sound inside the throttle window', () => {
        const view = restartOnce({ soundKey: 'alert', throttleSeconds: 10 });
        view.update({ pods: [pod('a', 1)] });
        view.flush();
        expect(mocks.oscillators).toBe(2);
        vi.setSystemTime(new Date('2024-05-01T10:00:05Z'));
        view.update({ pods: [pod('a', 2)] });
        view.flush();
        expect(added()).toHaveLength(2);
        expect(mocks.oscillators).toBe(2);
        vi.setSystemTime(new Date('2024-05-01T10:00:11Z'));
        view.update({ pods: [pod('a', 3)] });
        view.flush();
        expect(added()).toHaveLength(3);
        expect(mocks.oscillators).toBe(4);
    });
});

describe('usePodNotifications scale', () => {
    it('finds exactly the restarted pods among 5000', () => {
        const base = Array.from({ length: 5000 }, (_, i) => pod(`p-${i}`, 0, { ns: `ns-${i % 7}` }));
        const view = mount({ pods: base });
        const next = base.map((p, i) => (i % 1250 === 7 ? pod(`p-${i}`, 1, { ns: `ns-${i % 7}` }) : p));
        view.update({ pods: next });
        view.flush();
        expect(added().map(n => n.message)).toEqual([7, 1257, 2507, 3757].map(i => `p-${i} in ns-${i % 7} restarted — main (1)`));
    });
});
