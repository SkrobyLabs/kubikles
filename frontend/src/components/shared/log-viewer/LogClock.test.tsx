import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const harness = vi.hoisted(() => ({ states: [] as any[], index: 0, effects: [] as Array<() => (() => void)>, dialog: { current: { showModal: vi.fn(), close: vi.fn() } } }));
vi.mock('react', async original => ({
    ...await original<typeof import('react')>(),
    useState: (initial: any) => {
        const index = harness.index++;
        if (!(index in harness.states)) harness.states[index] = typeof initial === 'function' ? initial() : initial;
        return [harness.states[index], (value: any) => { harness.states[index] = value; }];
    },
    useRef: () => harness.dialog,
    useMemo: (factory: () => any) => factory(),
    useId: () => 'clock-test',
    useEffect: (effect: () => (() => void)) => harness.effects.push(effect),
}));
import LogClock from './LogClock';

function nodes(node: any): any[] {
    if (!node || typeof node !== 'object') return [];
    if (Array.isArray(node)) return node.flatMap(nodes);
    return [node, ...nodes(node.props?.children)];
}
function render() { harness.index = 0; harness.effects = []; return nodes(LogClock()); }

beforeEach(() => {
    harness.states = [];
    vi.clearAllMocks();
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2026-10-05T12:00:00Z'));
    vi.stubGlobal('window', { setInterval, clearInterval });
});
afterEach(() => { vi.useRealTimers(); vi.unstubAllGlobals(); });

describe('LogClock', () => {
    it('starts in UTC, ticks, and clears its timer on unmount', () => {
        let tree = render();
        expect(tree.find(n => n.type === 'time').props.children).toEqual(['05 Oct 2026, 12:00:00', ' · ', 'UTC']);
        expect(tree.find(n => n.props['aria-haspopup'] === 'dialog').props['aria-label']).toContain('05 Oct 2026, 12:00:00 UTC');
        const cleanup = harness.effects[0]();
        vi.advanceTimersByTime(1000);
        tree = render();
        expect(tree.find(n => n.type === 'time').props.children[0]).toBe('05 Oct 2026, 12:00:01');
        cleanup();
        expect(vi.getTimerCount()).toBe(0);
    });
    it('opens a searchable picker, selects a timezone and resets the filter on reopening', () => {
        let tree = render();
        tree.find(n => n.props['aria-haspopup'] === 'dialog').props.onClick();
        expect(harness.dialog.current.showModal).toHaveBeenCalledOnce();
        tree.find(n => n.type === 'input').props.onChange({ target: { value: 'kolkata' } });
        tree = render();
        const options = tree.filter(n => n.props['aria-pressed'] !== undefined);
        expect(options).toHaveLength(1);
        options[0].props.onClick();
        expect(harness.dialog.current.close).toHaveBeenCalledOnce();
        tree = render();
        expect(tree.find(n => n.type === 'time').props.children).toEqual(['05 Oct 2026, 17:30:00', ' · ', 'Asia/Kolkata']);
        tree.find(n => n.props['aria-haspopup'] === 'dialog').props.onClick();
        expect(render().find(n => n.type === 'input').props.value).toBe('');
    });
});
