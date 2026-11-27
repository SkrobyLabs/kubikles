import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// ResourceList keeps its sort, search, column-filter and unique-value logic inside the
// component body, so these tests invoke the real component function under a small hook
// runner (no DOM) and read the rows it hands to the virtualised table. State is driven the
// way the UI drives it: saved-view loading (query, sort, column filters), header clicks and
// the column-filter button. Nothing here depends on the order of hooks inside the component.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
// lag: true makes useDeferredValue keep returning its previous value, like a pending low-priority render.
const deferral = vi.hoisted(() => ({ lag: false }));
const mocks = vi.hoisted(() => ({
    savedView: { current: null as any },
    getDefaultView: () => null as any,
    loadView: () => null as any,
    TableVirtuoso: function TableVirtuoso() { return null; },
}));
mocks.getDefaultView = () => mocks.savedView.current;
mocks.loadView = () => mocks.savedView.current;

vi.mock('react', async () => {
    const actual: any = await vi.importActual('react');
    const memo = (factory: () => any, deps: any[]) => {
        const index = runner.index++;
        const old = runner.slots[index];
        if (!old || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) runner.slots[index] = { value: factory(), deps };
        return runner.slots[index].value;
    };
    const useEffect = (effect: () => any, deps?: any[]) => {
        const index = runner.index++;
        const old = runner.slots[index];
        if (!old || !deps || deps.some((dep, i) => !Object.is(dep, old.deps[i]))) {
            const slot = { deps, cleanup: undefined as any };
            runner.slots[index] = slot;
            runner.effects.push(() => { old?.cleanup?.(); slot.cleanup = effect(); });
        }
    };
    const patched = {
        ...actual,
        useMemo: memo,
        useDeferredValue: (value: any) => {
            const index = runner.index++;
            if (!runner.slots[index] || !deferral.lag) runner.slots[index] = { value };
            return runner.slots[index].value;
        },
        useCallback: (fn: any, deps: any[]) => memo(() => fn, deps),
        useRef: (initial: any) => memo(() => ({ current: initial }), []),
        useEffect,
        useLayoutEffect: useEffect,
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
    };
    return { ...patched, default: patched };
});
vi.mock('react-dom', () => ({ createPortal: (children: any) => children }));
vi.mock('react-virtuoso', () => ({ TableVirtuoso: mocks.TableVirtuoso }));
vi.mock('@dnd-kit/core', () => ({ DndContext: () => null, PointerSensor: class {}, useSensor: () => ({}), useSensors: () => [] }));
vi.mock('@dnd-kit/sortable', () => ({ arrayMove: (a: any[]) => a, SortableContext: () => null, horizontalListSortingStrategy: {} }));
vi.mock('wailsjs/runtime/runtime', () => ({ EventsOn: () => () => {} }));
vi.mock('~/context', () => ({
    useUI: () => ({ pendingSearch: null, consumePendingSearch: () => ({}) }),
    useConfig: () => ({ getConfig: () => undefined }),
}));
vi.mock('~/context/K8sContext', () => ({ useK8sActions: () => ({ refreshNamespaces: () => {} }) }));
vi.mock('~/hooks/useSavedViews', () => ({
    useSavedViews: () => ({
        views: [], saveView: vi.fn(), updateView: vi.fn(), deleteView: vi.fn(), renameView: vi.fn(),
        duplicateView: vi.fn(), setDefaultView: vi.fn(), loadView: mocks.loadView, getDefaultView: mocks.getDefaultView,
    }),
}));
vi.mock('./SearchSelect', () => ({ default: () => null }));
vi.mock('./BulkActionBar', () => ({ default: () => null }));
vi.mock('./ColumnConfigurator', () => ({ default: () => null }));
vi.mock('./RefreshControls', () => ({ default: () => null }));
vi.mock('./SavedViewsDropdown', () => ({ default: () => null }));
vi.mock('./ResourceLoadingStatus', () => ({ default: () => null }));
vi.mock('./ResourceListControls', () => ({
    SearchHighlightOverlay: () => null, TriStateCheckbox: () => null, RowCheckbox: () => null, SortableHeader: () => null,
}));

import ResourceList from './ResourceList';
import ResourceListHeader from './ResourceListHeader';

// ---- fixtures -------------------------------------------------------------------------------

const NOW = new Date('2024-06-01T12:00:00Z');
const hoursAgo = (hours: number) => new Date(NOW.getTime() - hours * 3600_000).toISOString();

interface PodSpec { name: string; ns?: string; age?: number | null; phase?: string; restarts?: number; node?: string; ts?: string }
const pod = ({ name, ns = 'default', age = 1, phase = 'Running', restarts = 0, node = '', ts }: PodSpec) => ({
    metadata: {
        name, namespace: ns, uid: `uid-${ns}-${name}`,
        ...(age === null ? {} : { creationTimestamp: ts ?? hoursAgo(age) }),
    },
    spec: { nodeName: node },
    status: { phase, containerStatuses: restarts ? [{ name: 'c', restartCount: restarts, state: { running: {} } }] : [] },
});

const STATUS_PRIORITY: Record<string, number> = { Failed: 1, CrashLoopBackOff: 2, Pending: 6, Running: 8, Succeeded: 9 };
const metrics: Record<string, { cpuPercent?: number; cpuCommittedPercent?: number }> = {};
const metricKey = (item: any) => `${item.metadata?.namespace}/${item.metadata?.name}`;

const columns: any[] = [
    { key: 'name', label: 'Name', render: (i: any) => i.metadata?.name, getValue: (i: any) => i.metadata?.name },
    { key: 'namespace', label: 'Namespace', render: (i: any) => i.metadata?.namespace, getValue: (i: any) => i.metadata?.namespace },
    {
        key: 'cpu', label: 'CPU', filterType: 'numeric', render: () => null,
        getValue: (i: any) => metrics[metricKey(i)]?.cpuCommittedPercent ?? -1,
        getNumericValue: (i: any) => metrics[metricKey(i)]?.cpuPercent ?? NaN,
    },
    { key: 'containers', label: 'Containers', filterable: false, render: () => null },
    {
        key: 'status', label: 'Status', render: () => null,
        getValue: (i: any) => STATUS_PRIORITY[i.status?.phase] ?? 10,
        getFilterValue: (i: any) => i.status?.phase,
    },
    {
        key: 'restarts', label: 'Restarts', filterType: 'numeric', render: () => null,
        getValue: (i: any) => (i.status?.containerStatuses || []).reduce((a: number, c: any) => a + c.restartCount, 0),
        getNumericValue: (i: any) => (i.status?.containerStatuses || []).reduce((a: number, c: any) => a + c.restartCount, 0),
    },
    {
        key: 'age', label: 'Age', filterType: 'numeric', render: () => null,
        getValue: (i: any) => i.metadata?.creationTimestamp,
        getNumericValue: (i: any) => (i.metadata?.creationTimestamp ? (Date.now() - new Date(i.metadata.creationTimestamp).getTime()) / 3600000 : NaN),
    },
    { key: 'node', label: 'Node', defaultHidden: true, render: () => null, getValue: (i: any) => i.spec?.nodeName || '' },
    { key: 'qos', label: 'QoS', render: () => null, getValue: (i: any) => i.status?.qosClass || '' },
];

// ---- harness --------------------------------------------------------------------------------

type Tree = any;
const walk = (node: Tree, visit: (el: any) => void) => {
    if (node == null || typeof node === 'boolean' || typeof node === 'string' || typeof node === 'number') return;
    if (Array.isArray(node)) { node.forEach(child => walk(child, visit)); return; }
    if (typeof node !== 'object' || !('props' in node)) return;
    visit(node);
    walk(node.props?.children, visit);
};
const findAll = (tree: Tree, predicate: (el: any) => boolean) => {
    const found: any[] = [];
    walk(tree, el => { if (predicate(el)) found.push(el); });
    return found;
};
const textOf = (node: Tree): string => {
    if (node == null || typeof node === 'boolean') return '';
    if (typeof node === 'string' || typeof node === 'number') return String(node);
    if (Array.isArray(node)) return node.map(textOf).join('');
    return textOf(node.props?.children);
};

// The header is its own memoised component, so its element must be rendered to see the
// <th> tree. Hooks inside it run in a reserved slot range so they never collide with the
// list's own hook slots.
const renderHeader = (element: any) => {
    if (element?.type !== ResourceListHeader) return element;
    const saved = runner.index;
    runner.index = 1000;
    try { return (ResourceListHeader as any).type(element.props); } finally { runner.index = saved; }
};

interface View {
    render(): Tree;
    setProps(next: Record<string, any>): View;
    table(): any;
    rows(): any[];
    names(): string[];
    hasTable(): boolean;
    headerTree(): any;
    clickHeader(key: string): View;
    sortIndicator(key: string): string;
    openFilter(key: string): View;
    dropdown(): Array<{ value: string; count: number }>;
    hasFilterButton(key: string): boolean;
}

function mount(data: any[], props: Record<string, any> = {}): View {
    const baseProps: Record<string, any> = {
        title: 'Pods', columns, data, isLoading: false, resourceType: 'pods', namespaces: [], currentNamespace: '*', ...props,
    };
    // Each mount is an independent component instance.
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = [];
    let tree: Tree;
    const render = () => {
        for (let i = 0; i < 25; i++) {
            runner.index = 0; runner.effects = [];
            tree = (ResourceList as any)(baseProps);
            runner.dirty = false;
            runner.effects.forEach(effect => effect());
            if (!runner.dirty) break;
        }
        return tree;
    };
    render();
    const table = () => findAll(tree, el => el.type === mocks.TableVirtuoso)[0];
    const api: View = {
        render,
        setProps: (next: Record<string, any>) => { Object.assign(baseProps, next); render(); return api; },
        table,
        rows: (): any[] => table()?.props.data ?? [],
        names: (): string[] => (table()?.props.data ?? []).map((row: any) => row.metadata.name),
        hasTable: () => Boolean(table()),
        headerTree: () => renderHeader(table()!.props.fixedHeaderContent()),
        // Clicking a sortable header runs the same handler as the UI.
        clickHeader: (key: string) => {
            const header = findAll(renderHeader(table()!.props.fixedHeaderContent()), el => el.props?.id === key && el.props?.className?.includes('cursor-pointer'))[0];
            const label = findAll(header, el => el.type === 'div' && typeof el.props.onClick === 'function')[0];
            label.props.onClick();
            render();
            return api;
        },
        sortIndicator: (key: string) => {
            const header = findAll(renderHeader(table()!.props.fixedHeaderContent()), el => el.props?.id === key && el.props?.className?.includes('cursor-pointer'))[0];
            return textOf(header).replace(/[A-Za-z]+/g, '').trim();
        },
        openFilter: (key: string) => {
            const header = findAll(renderHeader(table()!.props.fixedHeaderContent()), el => el.props?.id === key && el.props?.className?.includes('cursor-pointer'))[0];
            const button = findAll(header, el => el.type === 'button')[0];
            button.props.onClick({
                stopPropagation() {},
                currentTarget: { getBoundingClientRect: () => ({ left: 10, top: 10, bottom: 30 }) },
            });
            render();
            return api;
        },
        dropdown: () => {
            const labels = findAll(tree, el => el.type === 'label');
            return labels.map(label => {
                const spans = findAll(label, el => el.type === 'span');
                return { value: textOf(spans[0]), count: Number(textOf(spans[1])) };
            });
        },
        hasFilterButton: (key: string) => {
            const header = findAll(renderHeader(table()!.props.fixedHeaderContent()), el => el.props?.id === key && el.props?.className?.includes('cursor-pointer'))[0];
            return Boolean(header) && findAll(header, el => el.type === 'button').length > 0;
        },
    };
    return api;
}

/** Loads a saved view on mount, exactly as the default-view effect does. */
const withView = (view: Record<string, any>) => {
    mocks.savedView.current = { id: 'view-1', query: '', ...view };
};
const sortBy = (key: string | null, direction = 'asc') => ({ sortConfig: { key, direction } });

beforeEach(() => {
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    mocks.savedView.current = null;
    deferral.lag = false;
    for (const key of Object.keys(metrics)) delete metrics[key];
    vi.useFakeTimers({ toFake: ['Date'] });
    vi.setSystemTime(NOW);
    const g = globalThis as any;
    g.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {} };
    g.window = { innerWidth: 1200, innerHeight: 800 };
    g.document = { body: {}, addEventListener: () => {}, removeEventListener: () => {} };
});
afterEach(() => {
    vi.useRealTimers();
    // Release the row heartbeat interval of the last mounted instance.
    runner.slots.forEach(slot => slot?.cleanup?.());
});

// ---- sorting --------------------------------------------------------------------------------

describe('ResourceList sorting', () => {
    const trio = [
        pod({ name: 'b-mid', age: 5 }),
        pod({ name: 'a-old', age: 50 }),
        pod({ name: 'c-new', age: 1 }),
    ];

    it('keeps incoming order when there is no sort', () => {
        expect(mount(trio).names()).toEqual(['b-mid', 'a-old', 'c-new']);
    });

    it('sorts by age descending (newest first) and ascending (oldest first) on the raw timestamp', () => {
        expect(mount(trio, { initialSort: { key: 'age', direction: 'desc' } }).names()).toEqual(['c-new', 'b-mid', 'a-old']);
        expect(mount(trio, { initialSort: { key: 'age', direction: 'asc' } }).names()).toEqual(['a-old', 'b-mid', 'c-new']);
    });

    it('breaks age ties by name ascending, whatever the sort direction', () => {
        const same = hoursAgo(3);
        const tied = [pod({ name: 'zeta', ts: same }), pod({ name: 'alpha', ts: same }), pod({ name: 'mike', ts: same })];
        expect(mount(tied, { initialSort: { key: 'age', direction: 'desc' } }).names()).toEqual(['alpha', 'mike', 'zeta']);
        expect(mount(tied, { initialSort: { key: 'age', direction: 'asc' } }).names()).toEqual(['alpha', 'mike', 'zeta']);
    });

    it('sorts by name in both directions without a further tie-break', () => {
        expect(mount(trio, { initialSort: { key: 'name', direction: 'asc' } }).names()).toEqual(['a-old', 'b-mid', 'c-new']);
        expect(mount(trio, { initialSort: { key: 'name', direction: 'desc' } }).names()).toEqual(['c-new', 'b-mid', 'a-old']);
    });

    it('compares strings with plain code-unit ordering (upper case before lower case)', () => {
        const mixed = [pod({ name: 'apple' }), pod({ name: 'Zebra' }), pod({ name: 'Banana' })];
        expect(mount(mixed, { initialSort: { key: 'name', direction: 'asc' } }).names()).toEqual(['Banana', 'Zebra', 'apple']);
    });

    it('compares numbers numerically but numeric-looking strings lexically', () => {
        const restarts = [pod({ name: 'p10', restarts: 10 }), pod({ name: 'p9', restarts: 9 }), pod({ name: 'p100', restarts: 100 })];
        expect(mount(restarts, { initialSort: { key: 'restarts', direction: 'asc' } }).names()).toEqual(['p9', 'p10', 'p100']);
        const stringy = [{ key: 'v', label: 'V', getValue: (i: any) => i.v }];
        const items = ['10', '9', '100'].map(v => ({ metadata: { name: `n${v}`, uid: `u${v}` }, v }));
        const view = mount(items, { columns: stringy, initialSort: { key: 'v', direction: 'asc' } });
        expect(view.names()).toEqual(['n10', 'n100', 'n9']);
    });

    it('sorts by status priority and then falls back to age (newest first) and name', () => {
        const pods = [
            pod({ name: 'run-old', phase: 'Running', age: 20 }),
            pod({ name: 'fail', phase: 'Failed', age: 30 }),
            pod({ name: 'run-new-b', phase: 'Running', age: 2 }),
            pod({ name: 'run-new-a', phase: 'Running', age: 2, ts: hoursAgo(2) }),
            pod({ name: 'pending', phase: 'Pending', age: 4 }),
        ];
        expect(mount(pods, { initialSort: { key: 'status', direction: 'asc' } }).names())
            .toEqual(['fail', 'pending', 'run-new-a', 'run-new-b', 'run-old']);
        expect(mount(pods, { initialSort: { key: 'status', direction: 'desc' } }).names())
            .toEqual(['run-new-a', 'run-new-b', 'run-old', 'pending', 'fail']);
    });

    it('sorts by a metrics-derived value, placing rows without metrics (-1) first ascending and last descending', () => {
        const pods = [pod({ name: 'hot' }), pod({ name: 'idle' }), pod({ name: 'nometrics' }), pod({ name: 'warm' })];
        metrics['default/hot'] = { cpuCommittedPercent: 90 };
        metrics['default/idle'] = { cpuCommittedPercent: 0 };
        metrics['default/warm'] = { cpuCommittedPercent: 45.5 };
        expect(mount(pods, { initialSort: { key: 'cpu', direction: 'asc' } }).names()).toEqual(['nometrics', 'idle', 'warm', 'hot']);
        expect(mount(pods, { initialSort: { key: 'cpu', direction: 'desc' } }).names()).toEqual(['hot', 'warm', 'idle', 'nometrics']);
    });

    it('treats rows with identical metric values as ties and orders them by age then name', () => {
        const same = hoursAgo(4);
        const pods = [pod({ name: 'b', ts: same }), pod({ name: 'a', ts: same }), pod({ name: 'newer', age: 1 })];
        for (const p of pods) metrics[`default/${p.metadata.name}`] = { cpuCommittedPercent: 10 };
        expect(mount(pods, { initialSort: { key: 'cpu', direction: 'asc' } }).names()).toEqual(['newer', 'a', 'b']);
    });

    it('orders null/undefined sort values: null counts as 0 against numbers', () => {
        const vcol = [{ key: 'v', label: 'V', getValue: (i: any) => i.v }, columns[0]];
        const items = [{ v: 2, name: 'two' }, { v: null, name: 'nil' }, { v: 1, name: 'one' }]
            .map(o => ({ v: o.v, metadata: { name: o.name, uid: o.name } }));
        expect(mount(items, { columns: vcol, initialSort: { key: 'v', direction: 'asc' } }).names()).toEqual(['nil', 'one', 'two']);
        expect(mount(items, { columns: vcol, initialSort: { key: 'v', direction: 'desc' } }).names()).toEqual(['two', 'one', 'nil']);
    });

    it('treats all-undefined sort values as ties resolved by age then name', () => {
        const vcol = [{ key: 'v', label: 'V', getValue: () => undefined }, columns[0]];
        const items = [pod({ name: 'b', age: 5 }), pod({ name: 'a', age: 5, ts: hoursAgo(5) }), pod({ name: 'c', age: 1 })];
        expect(mount(items, { columns: vcol, initialSort: { key: 'v', direction: 'asc' } }).names()).toEqual(['c', 'a', 'b']);
    });

    it('handles items without a creationTimestamp when the tie-break needs one', () => {
        const items = [pod({ name: 'zz', age: null, phase: 'Running' }), pod({ name: 'aa', age: null, phase: 'Running' })];
        expect(mount(items, { initialSort: { key: 'status', direction: 'asc' } }).names()).toEqual(['aa', 'zz']);
    });

    it('does not reorder or mutate the incoming data array', () => {
        const input = [...trio];
        mount(input, { initialSort: { key: 'name', direction: 'asc' } });
        expect(input.map(p => p.metadata.name)).toEqual(['b-mid', 'a-old', 'c-new']);
    });

    it('leaves rows in incoming order for an unknown sort key', () => {
        expect(mount(trio, { initialSort: { key: 'does-not-exist', direction: 'asc' } }).names()).toEqual(['b-mid', 'a-old', 'c-new']);
    });

    it('is stable for fully tied rows (same age and name)', () => {
        const twin = (uid: string) => ({ ...pod({ name: 'twin', ts: hoursAgo(1) }), metadata: { name: 'twin', namespace: 'default', uid, creationTimestamp: hoursAgo(1) } });
        const view = mount([twin('first'), twin('second'), twin('third')], { initialSort: { key: 'name', direction: 'asc' } });
        expect(view.rows().map(r => r.metadata.uid)).toEqual(['first', 'second', 'third']);
    });

    it('toggles direction via header clicks: first click ascending, second descending, third ascending', () => {
        const view = mount(trio);
        expect(view.clickHeader('name').names()).toEqual(['a-old', 'b-mid', 'c-new']);
        expect(view.sortIndicator('name')).toBe('↑');
        view.clickHeader('name');
        expect(view.names()).toEqual(['c-new', 'b-mid', 'a-old']);
        expect(view.sortIndicator('name')).toBe('↓');
        view.clickHeader('name');
        expect(view.names()).toEqual(['a-old', 'b-mid', 'c-new']);
        // Switching columns always starts ascending.
        view.clickHeader('age');
        expect(view.names()).toEqual(['a-old', 'b-mid', 'c-new']);
        expect(view.sortIndicator('age')).toBe('↑');
        expect(view.sortIndicator('name')).toBe('');
    });

    it('a saved view without a sort clears the initial sort', () => {
        withView({ columnFilters: {} });
        expect(mount(trio, { initialSort: { key: 'name', direction: 'asc' } }).names()).toEqual(['b-mid', 'a-old', 'c-new']);
    });
});

// ---- search ---------------------------------------------------------------------------------

describe('ResourceList search filtering', () => {
    const pods = [
        pod({ name: 'nginx-1', ns: 'web', phase: 'Running', node: 'node-a' }),
        pod({ name: 'nginx-2', ns: 'web', phase: 'Pending', node: 'node-b' }),
        pod({ name: 'redis-1', ns: 'data', phase: 'Running', node: 'node-a' }),
        pod({ name: 'api', ns: 'web', phase: 'Failed', node: 'node-c' }),
    ];

    it('shows everything for an empty query', () => {
        withView({ query: '', ...sortBy('name') });
        expect(mount(pods).names()).toEqual(['api', 'nginx-1', 'nginx-2', 'redis-1']);
    });

    it('matches plain text against the name, case-insensitively', () => {
        withView({ query: 'NGINX', ...sortBy('name') });
        expect(mount(pods).names()).toEqual(['nginx-1', 'nginx-2']);
    });

    it('supports field queries with aliases, AND within a group and OR between groups', () => {
        withView({ query: 'ns:web status:Running', ...sortBy('name') });
        expect(mount(pods).names()).toEqual(['nginx-1']);
        withView({ query: 'node:node-a OR name:api', ...sortBy('name') });
        expect(mount(pods).names()).toEqual(['api', 'nginx-1', 'redis-1']);
    });

    it('supports regular expressions and shows nothing when nothing matches', () => {
        withView({ query: 'name:/^(redis|api)/', ...sortBy('name') });
        expect(mount(pods).names()).toEqual(['api', 'redis-1']);
        withView({ query: 'zzz', ...sortBy('name') });
        const view = mount(pods);
        expect(view.names()).toEqual([]);
        expect(view.hasTable()).toBe(false);
    });

    it('uses the resource type to resolve fields; unknown fields match nothing', () => {
        withView({ query: 'nodename:node-b', ...sortBy('name') });
        expect(mount(pods).names()).toEqual(['nginx-2']);
        withView({ query: 'bogus:x', ...sortBy('name') });
        expect(mount(pods).names()).toEqual([]);
    });

    it('applies the search before sorting and keeps the sort inside the filtered set', () => {
        withView({ query: 'ns:web', ...sortBy('name', 'desc') });
        expect(mount(pods).names()).toEqual(['nginx-2', 'nginx-1', 'api']);
    });

    it('reports the filtered uids to the parent only when the set changes', () => {
        const onFilteredUidsChange = vi.fn();
        withView({ query: 'ns:web', ...sortBy('name') });
        const view = mount(pods, { onFilteredUidsChange });
        // The first report precedes the saved view being applied (all rows); the second is the filtered set.
        expect(onFilteredUidsChange).toHaveBeenCalledTimes(2);
        expect(onFilteredUidsChange.mock.calls[0][0].size).toBe(4);
        expect([...onFilteredUidsChange.mock.calls[1][0]].sort()).toEqual(['uid-web-api', 'uid-web-nginx-1', 'uid-web-nginx-2']);
        view.render();
        expect(onFilteredUidsChange).toHaveBeenCalledTimes(2);
    });
});

// ---- column filters -------------------------------------------------------------------------

describe('ResourceList deferred pipeline', () => {
    it('keeps showing the previous rows while an ordinary update is deferred', () => {
        const view = mount([pod({ name: 'a' }), pod({ name: 'b' })]);
        deferral.lag = true;
        view.setProps({ data: [pod({ name: 'a' })] });
        expect(view.names()).toEqual(['a', 'b']);
    });

    it('drops stale rows immediately when the data resets to empty (context or scope switch)', () => {
        const view = mount([pod({ name: 'a' }), pod({ name: 'b' })]);
        deferral.lag = true;
        view.setProps({ data: [] });
        expect(view.names()).toEqual([]);
    });
});

describe('ResourceList column filters', () => {
    const pods = [
        pod({ name: 'web-1', ns: 'shop', phase: 'Running', restarts: 0, age: 2, node: 'node-a' }),
        pod({ name: 'web-2', ns: 'shop', phase: 'Pending', restarts: 3, age: 30, node: 'node-b' }),
        pod({ name: 'db-0', ns: 'data', phase: 'Running', restarts: 12, age: 100, node: 'node-a' }),
        pod({ name: 'Job-X', ns: 'batch', phase: 'Failed', restarts: 1, age: 0.5, node: '' }),
    ];
    const filtered = (columnFilters: Record<string, any>, extra: Record<string, any> = {}) => {
        withView({ ...sortBy('name'), columnFilters, ...extra });
        return mount(pods).names();
    };

    it('select filters use the filter value, accept several values and reject empty selections', () => {
        expect(filtered({ status: { type: 'select', values: ['Running'] } })).toEqual(['db-0', 'web-1'].sort());
        expect(filtered({ status: { type: 'select', values: ['Running', 'Failed'] } })).toEqual(['Job-X', 'db-0', 'web-1']);
        // QUIRK: a restored select filter with no values hides every row.
        expect(filtered({ status: { type: 'select', values: [] } })).toEqual([]);
    });

    it('select filters on a column that exposes only getValue compare its string value', () => {
        expect(filtered({ node: { type: 'select', values: ['node-a'] } })).toEqual(['db-0', 'web-1']);
        // Hidden-by-default columns still filter.
        expect(filtered({ node: { type: 'select', values: [''] } })).toEqual(['Job-X']);
    });

    it('regex filters are case-insensitive, AND by default, OR on request, and use the name/namespace metadata', () => {
        expect(filtered({ name: { type: 'regex', conditions: ['^web'] } })).toEqual(['web-1', 'web-2']);
        expect(filtered({ name: { type: 'regex', conditions: ['job'] } })).toEqual(['Job-X']);
        expect(filtered({ name: { type: 'regex', conditions: ['web', '-2$'] } })).toEqual(['web-2']);
        expect(filtered({ name: { type: 'regex', conditions: ['web', '^db'], logic: 'or' } })).toEqual(['db-0', 'web-1', 'web-2']);
        expect(filtered({ namespace: { type: 'regex', conditions: ['^(data|batch)$'] } })).toEqual(['Job-X', 'db-0']);
    });

    it('regex filters ignore invalid patterns and accept the legacy single-pattern field', () => {
        expect(filtered({ name: { type: 'regex', conditions: ['(['] } })).toEqual(['Job-X', 'db-0', 'web-1', 'web-2']);
        expect(filtered({ name: { type: 'regex', pattern: '^db' } })).toEqual(['db-0']);
    });

    it('numeric filters support every operator on the numeric value', () => {
        const restarts = (operator: string, value: number) => filtered({ restarts: { type: 'numeric', conditions: [{ operator, value }] } });
        expect(restarts('>', 1)).toEqual(['db-0', 'web-2']);
        expect(restarts('>=', 1)).toEqual(['Job-X', 'db-0', 'web-2']);
        expect(restarts('<', 1)).toEqual(['web-1']);
        expect(restarts('<=', 1)).toEqual(['Job-X', 'web-1']);
        expect(restarts('=', 3)).toEqual(['web-2']);
        expect(restarts('!=', 3)).toEqual(['Job-X', 'db-0', 'web-1']);
        // An unknown operator never rejects.
        expect(restarts('~', 3)).toEqual(['Job-X', 'db-0', 'web-1', 'web-2']);
    });

    it('numeric filters combine conditions with AND by default and OR on request, and accept the legacy shape', () => {
        const both = [{ operator: '>', value: 0 }, { operator: '<', value: 5 }];
        expect(filtered({ restarts: { type: 'numeric', conditions: both } })).toEqual(['Job-X', 'web-2']);
        expect(filtered({ restarts: { type: 'numeric', conditions: [{ operator: '<', value: 1 }, { operator: '>', value: 10 }], logic: 'or' } }))
            .toEqual(['db-0', 'web-1']);
        expect(filtered({ restarts: { type: 'numeric', operator: '>=', value: 3 } })).toEqual(['db-0', 'web-2']);
    });

    it('age filters work in hours from the current time', () => {
        expect(filtered({ age: { type: 'numeric', conditions: [{ operator: '>', value: 24 }] } })).toEqual(['db-0', 'web-2']);
        expect(filtered({ age: { type: 'numeric', conditions: [{ operator: '<', value: 1 }] } })).toEqual(['Job-X']);
    });

    it('rows whose numeric value is NaN never pass a numeric filter', () => {
        metrics['shop/web-1'] = { cpuPercent: 50 };
        metrics['data/db-0'] = { cpuPercent: 5 };
        expect(filtered({ cpu: { type: 'numeric', conditions: [{ operator: '>=', value: 0 }] } })).toEqual(['db-0', 'web-1']);
        expect(filtered({ cpu: { type: 'numeric', conditions: [{ operator: '!=', value: 50 }] } })).toEqual(['db-0']);
    });

    it('a filter for a column that does not exist is ignored', () => {
        expect(filtered({ nonexistent: { type: 'select', values: ['x'] } })).toEqual(['Job-X', 'db-0', 'web-1', 'web-2']);
    });

    it('multiple column filters are ANDed and combined with the search query', () => {
        expect(filtered({
            status: { type: 'select', values: ['Running'] },
            namespace: { type: 'regex', conditions: ['shop'] },
        })).toEqual(['web-1']);
        expect(filtered({ restarts: { type: 'numeric', conditions: [{ operator: '>', value: 0 }] } }, { query: 'ns:shop' })).toEqual(['web-2']);
    });

    it('a column without getFilterValue or string getValue is filtered on its raw field or an empty string', () => {
        const numericCol = [columns[0], { key: 'weight', label: 'Weight', render: () => null, getValue: (i: any) => i.weight }];
        const items = [{ weight: 5, metadata: { name: 'a', uid: 'a' } }, { weight: 7, metadata: { name: 'b', uid: 'b' } }];
        withView({ ...sortBy('name'), columnFilters: { weight: { type: 'regex', conditions: ['^$'] } } });
        // getValue returns a number, which counts as no string value at all.
        expect(mount(items, { columns: numericCol }).names()).toEqual(['a', 'b']);
        withView({ ...sortBy('name'), columnFilters: { weight: { type: 'regex', conditions: ['5'] } } });
        expect(mount(items, { columns: numericCol }).names()).toEqual([]);
    });

    it('QUIRK: a saved view that omits hiddenColumns shows every column, including default-hidden ones', () => {
        withView({ ...sortBy('name'), columnFilters: {} });
        const view = mount(pods);
        const keys = findAll(view.headerTree(), el => el.props?.id && el.props?.className?.includes('cursor-pointer')).map(el => el.props.id);
        expect(keys).toContain('node');
        mocks.savedView.current = null;
        const plain = mount(pods);
        const plainKeys = findAll(plain.headerTree(), el => el.props?.id && el.props?.className?.includes('cursor-pointer')).map(el => el.props.id);
        expect(plainKeys).not.toContain('node');
    });
});

// ---- unique values for select columns -------------------------------------------------------

describe('ResourceList select-column unique values', () => {
    const mixed = [
        pod({ name: 'a', phase: 'Running' }), pod({ name: 'b', phase: 'Running' }), pod({ name: 'c', phase: 'Running' }),
        pod({ name: 'd', phase: 'Pending' }), pod({ name: 'e', phase: 'Failed' }),
        pod({ name: 'f', phase: 'Zeta' }), pod({ name: 'g', phase: 'Alpha' }), pod({ name: 'h', phase: 'Alpha' }),
    ];

    it('lists predefined values first in their declared order (zero counts included), then dynamic values alphabetically', () => {
        const view = mount(mixed);
        view.openFilter('status');
        const entries = view.dropdown();
        const predefined = ['Running', 'Pending', 'Failed', 'Succeeded', 'CrashLoopBackOff', 'Terminating', 'Unknown',
            'ErrImagePull', 'ImagePullBackOff', 'ContainerCreating', 'PodInitializing', 'Init:Error', 'Init:CrashLoopBackOff', 'Init:Running'];
        expect(entries.map(e => e.value)).toEqual([...predefined, 'Alpha', 'Zeta']);
        const counts = Object.fromEntries(entries.map(e => [e.value, e.count]));
        expect(counts).toMatchObject({ Running: 3, Pending: 1, Failed: 1, Succeeded: 0, Terminating: 0, Alpha: 2, Zeta: 1 });
    });

    it('counts over all rows of the data, not just the filtered or searched ones', () => {
        withView({ ...sortBy('name'), query: 'name:a', columnFilters: {} });
        const view = mount(mixed);
        view.openFilter('status');
        expect(view.names()).toEqual(['a']);
        expect(Object.fromEntries(view.dropdown().map(e => [e.value, e.count])).Running).toBe(3);
    });

    it('derives values entirely from data when the resource type has no predefined list', () => {
        const view = mount(mixed, { resourceType: 'widgets' });
        view.openFilter('status');
        expect(view.dropdown()).toEqual([
            { value: 'Alpha', count: 2 }, { value: 'Failed', count: 1 }, { value: 'Pending', count: 1 },
            { value: 'Running', count: 3 }, { value: 'Zeta', count: 1 },
        ]);
    });

    it('omits empty values and offers no filter when a select column has no data and no predefined list', () => {
        const view = mount(mixed);
        expect(view.hasFilterButton('qos')).toBe(false);
        expect(view.hasFilterButton('status')).toBe(true);
    });

    it('offers filter buttons by detected type: regex for name/namespace, numeric for age, none when filterable is false', () => {
        const view = mount(mixed);
        expect(view.hasFilterButton('name')).toBe(true);
        expect(view.hasFilterButton('namespace')).toBe(true);
        expect(view.hasFilterButton('age')).toBe(true);
        expect(view.hasFilterButton('restarts')).toBe(true);
        expect(view.hasFilterButton('containers')).toBe(false);
    });

    it('suppresses the filter for a select column with more than 200 distinct values', () => {
        const many = Array.from({ length: 201 }, (_, i) => pod({ name: `p${i}`, phase: `Phase-${i}` }));
        expect(mount(many, { resourceType: 'widgets' }).hasFilterButton('status')).toBe(false);
        const edge = Array.from({ length: 200 }, (_, i) => pod({ name: `p${i}`, phase: `Phase-${i}` }));
        const view = mount(edge, { resourceType: 'widgets' });
        expect(view.hasFilterButton('status')).toBe(true);
        view.openFilter('status');
        expect(view.dropdown()).toHaveLength(200);
    });

    it('uses the per-type predefined lists for other resource types', () => {
        const nodeCols = [{ key: 'status', label: 'Status', render: () => null, getFilterValue: (i: any) => i.s }, columns[0]];
        const nodes = [{ s: 'Ready', metadata: { name: 'n1', uid: 'n1' } }, { s: 'Weird', metadata: { name: 'n2', uid: 'n2' } }];
        const view = mount(nodes, { columns: nodeCols, resourceType: 'nodes' });
        view.openFilter('status');
        expect(view.dropdown()).toEqual([
            { value: 'Ready', count: 1 }, { value: 'NotReady', count: 0 }, { value: 'SchedulingDisabled', count: 0 }, { value: 'Weird', count: 1 },
        ]);
    });
});

// ---- scale ----------------------------------------------------------------------------------

describe('ResourceList at 5000 rows', () => {
    const PHASES = ['Running', 'Running', 'Running', 'Pending', 'Failed', 'Succeeded'];
    const big = Array.from({ length: 5000 }, (_, i) => pod({
        name: `pod-${String(i).padStart(4, '0')}`,
        ns: `ns-${i % 25}`,
        // Many rows share an age to exercise the tie-breaks.
        ts: new Date(NOW.getTime() - (i % 100) * 3600_000).toISOString(),
        phase: PHASES[i % PHASES.length],
        restarts: i % 17 === 0 ? i % 5 : 0,
    }));

    it('default age-descending sort is a total order: age descending, name ascending within an age', () => {
        const rows = mount(big, { initialSort: { key: 'age', direction: 'desc' } }).rows();
        expect(rows).toHaveLength(5000);
        for (let i = 1; i < rows.length; i++) {
            const prev = rows[i - 1].metadata, cur = rows[i].metadata;
            if (prev.creationTimestamp === cur.creationTimestamp) expect(prev.name < cur.name).toBe(true);
            else expect(prev.creationTimestamp > cur.creationTimestamp).toBe(true);
        }
        expect(new Set(rows.map(r => r.metadata.uid)).size).toBe(5000);
    });

    it('status sort groups by priority with the documented tie-breaks', () => {
        const rows = mount(big, { initialSort: { key: 'status', direction: 'asc' } }).rows();
        const priorities = rows.map(r => STATUS_PRIORITY[r.status.phase]);
        expect(priorities).toEqual([...priorities].sort((a, b) => a - b));
        const firstRunning = rows.findIndex(r => r.status.phase === 'Running');
        const runningRows = rows.slice(firstRunning, firstRunning + 2500);
        expect(runningRows.every(r => r.status.phase === 'Running')).toBe(true);
        for (let i = 1; i < runningRows.length; i++) {
            const prev = runningRows[i - 1].metadata, cur = runningRows[i].metadata;
            if (prev.creationTimestamp === cur.creationTimestamp) expect(prev.name < cur.name).toBe(true);
            else expect(prev.creationTimestamp > cur.creationTimestamp).toBe(true);
        }
    });

    it('search + select + regex + numeric filters yield exactly the expected subset in sorted order', () => {
        withView({
            ...sortBy('name'),
            query: 'ns:/^ns-(3|4)$/',
            columnFilters: {
                status: { type: 'select', values: ['Running', 'Failed'] },
                name: { type: 'regex', conditions: ['[02468]$'] },
                restarts: { type: 'numeric', conditions: [{ operator: '=', value: 0 }] },
            },
        });
        const expected = big
            .filter((p, i) => {
                const n = i % 25;
                const status = PHASES[i % PHASES.length];
                const restarts = i % 17 === 0 ? i % 5 : 0;
                return (n === 3 || n === 4) && (status === 'Running' || status === 'Failed') && /[02468]$/.test(p.metadata.name) && restarts === 0;
            })
            .map(p => p.metadata.name)
            .sort();
        expect(expected.length).toBeGreaterThan(50);
        expect(mount(big).names()).toEqual(expected);
    });

    it('computes status counts across all 5000 rows', () => {
        const view = mount(big);
        view.openFilter('status');
        const counts = Object.fromEntries(view.dropdown().map(e => [e.value, e.count]));
        expect(counts.Running).toBe(big.filter(p => p.status.phase === 'Running').length);
        expect(counts.Pending + counts.Failed + counts.Succeeded + counts.Running).toBe(5000);
    });

    it('metrics sort over 5000 rows is monotonic with missing metrics first', () => {
        big.forEach((p, i) => { if (i % 3) metrics[metricKey(p)] = { cpuCommittedPercent: (i * 7) % 101 }; });
        const rows = mount(big, { initialSort: { key: 'cpu', direction: 'asc' } }).rows();
        const values = rows.map(r => metrics[metricKey(r)]?.cpuCommittedPercent ?? -1);
        expect(values).toEqual([...values].sort((a, b) => a - b));
        expect(values[0]).toBe(-1);
    });
});


// ---- render-cost behaviour ------------------------------------------------------------------

describe('ResourceList render cost', () => {
    const trio = [
        pod({ name: 'b', age: 5 }),
        pod({ name: 'a', age: 50 }),
        pod({ name: 'c', age: 1 }),
    ];

    it('keys rows by uid, falling back to namespace/name and then index', () => {
        const view = mount(trio);
        const key = view.table().props.computeItemKey;
        expect(key(0, trio[0])).toBe('uid-default-b');
        expect(key(3, { name: 'rel', namespace: 'prod' })).toBe('prod/rel');
        expect(key(7, {})).toBe(7);
    });

    it('overscans enough to avoid blank rows on fast scroll', () => {
        expect(mount(trio).table().props.overscan).toBeGreaterThanOrEqual(200);
    });

    it('passes stable header and row callbacks so a data-only change does not rebuild them', () => {
        const view = mount(trio);
        const header = view.table().props.fixedHeaderContent;
        const itemContent = view.table().props.itemContent;
        const context = view.table().props.context;
        // same size: widths are only recalculated when the row count moves by more than 10%
        view.setProps({ data: trio.map(p => ({ ...p, status: { ...p.status, phase: 'Pending' } })) });
        expect(view.table().props.fixedHeaderContent).toBe(header);
        expect(view.table().props.itemContent).toBe(itemContent);
        expect(view.table().props.context).toBe(context);
    });

    it('does not rebuild the table context when the parent recreates onRowClick every render', () => {
        const view = mount(trio, { onRowClick: () => {} });
        const context = view.table().props.context;
        view.setProps({ onRowClick: () => {} });
        expect(view.table().props.context).toBe(context);
    });

    it('does not scan or build select values until a dropdown is opened, and never for hidden columns', () => {
        const statusCalls = vi.fn((i: any) => i.status?.phase);
        const hiddenCalls = vi.fn((i: any) => i.spec?.nodeName || '');
        const cols = columns.map(c => c.key === 'status' ? { ...c, getFilterValue: statusCalls }
            : c.key === 'node' ? { ...c, getFilterValue: hiddenCalls } : c);
        const view = mount(trio, { columns: cols });
        // 'pods' status has predefined values, so its funnel needs no scan; the hidden column is not looked at.
        expect(statusCalls).not.toHaveBeenCalled();
        expect(hiddenCalls).not.toHaveBeenCalled();
        expect(view.hasFilterButton('status')).toBe(true);
        view.openFilter('status');
        expect(statusCalls).toHaveBeenCalledTimes(trio.length);
        expect(view.dropdown().find(v => v.value === 'Running')?.count).toBe(3);
    });

    it('refreshes the open dropdown counts when data changes', () => {
        const view = mount(trio).openFilter('status');
        expect(view.dropdown().find(v => v.value === 'Running')?.count).toBe(3);
        view.setProps({ data: [...trio, pod({ name: 'd', phase: 'Failed' })] });
        expect(view.dropdown().find(v => v.value === 'Failed')?.count).toBe(1);
    });

    it('reports filtered uids only when the set really changes', () => {
        const onFilteredUidsChange = vi.fn();
        const view = mount(trio, { onFilteredUidsChange });
        expect(onFilteredUidsChange).toHaveBeenCalledTimes(1);
        // same uids, new objects (a watch batch that rewrote every item)
        view.setProps({ data: trio.map(p => ({ ...p })) });
        expect(onFilteredUidsChange).toHaveBeenCalledTimes(1);
        view.setProps({ data: [...trio, pod({ name: 'd' })] });
        expect(onFilteredUidsChange).toHaveBeenCalledTimes(2);
        expect([...onFilteredUidsChange.mock.calls[1][0]].sort()).toEqual(['uid-default-a', 'uid-default-b', 'uid-default-c', 'uid-default-d']);
    });
});

describe('ResourceList volatile columns', () => {
    const volatileCols = columns.map(c => (c.key === 'cpu' ? { ...c, volatile: true } : c));
    const rows3 = [pod({ name: 'a' }), pod({ name: 'b' }), pod({ name: 'c' })];

    it('re-sorts on a metrics tick only while sorted by a volatile column', () => {
        metrics['default/a'] = { cpuCommittedPercent: 10, cpuPercent: 10 };
        metrics['default/b'] = { cpuCommittedPercent: 20, cpuPercent: 20 };
        metrics['default/c'] = { cpuCommittedPercent: 30, cpuPercent: 30 };
        withView(sortBy('cpu', 'asc'));
        const view = mount(rows3, { columns: volatileCols, volatileInputs: { tick: 1 } });
        expect(view.names()).toEqual(['a', 'b', 'c']);
        metrics['default/a'] = { cpuCommittedPercent: 99, cpuPercent: 99 };
        view.setProps({ volatileInputs: { tick: 2 } });
        expect(view.names()).toEqual(['b', 'c', 'a']);
    });

    it('does not re-filter or re-sort on a metrics tick when sorted by another column', () => {
        withView(sortBy('name', 'asc'));
        const view = mount(rows3, { columns: volatileCols, volatileInputs: { tick: 1 } });
        const before = view.rows();
        view.setProps({ volatileInputs: { tick: 2 } });
        expect(view.rows()).toBe(before);
    });

    it('re-applies a numeric filter on a volatile column when metrics change', () => {
        metrics['default/a'] = { cpuPercent: 10 };
        metrics['default/b'] = { cpuPercent: 20 };
        metrics['default/c'] = { cpuPercent: 30 };
        withView({ ...sortBy('name', 'asc'), columnFilters: { cpu: { type: 'numeric', conditions: [{ operator: '>', value: 15 }], logic: 'and' } } });
        const view = mount(rows3, { columns: volatileCols, volatileInputs: { tick: 1 } });
        expect(view.names()).toEqual(['b', 'c']);
        metrics['default/a'] = { cpuPercent: 50 };
        view.setProps({ volatileInputs: { tick: 2 } });
        expect(view.names()).toEqual(['a', 'b', 'c']);
    });
});
