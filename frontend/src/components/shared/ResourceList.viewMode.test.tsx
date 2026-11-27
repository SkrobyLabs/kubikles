import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// View mode of ResourceList: the rows come from a backend view handle, and sorting, filtering
// and searching are pushed to it instead of running in the component. The same tiny hook runner
// as ResourceList.pipeline.test.tsx drives the real component function without a DOM.
const runner = vi.hoisted(() => ({ slots: [] as any[], index: 0, effects: [] as Array<() => void>, dirty: false }));
const mocks = vi.hoisted(() => ({
    savedView: { current: null as any },
    getDefaultView: () => null as any,
    loadView: () => null as any,
    TableVirtuoso: function TableVirtuoso() { return null; },
    BulkActionBar: function BulkActionBar() { return null; },
    createFilter: vi.fn(),
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
        useDeferredValue: (value: any) => value,
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
    useConfig: () => ({ getConfig: (key: string) => (key === 'ui.searchDebounceMs' ? 0 : undefined) }),
}));
vi.mock('~/context/K8sContext', () => ({ useK8sActions: () => ({ refreshNamespaces: () => {} }) }));
vi.mock('~/hooks/useSavedViews', () => ({
    useSavedViews: () => ({
        views: [], saveView: vi.fn(), updateView: vi.fn(), deleteView: vi.fn(), renameView: vi.fn(),
        duplicateView: vi.fn(), setDefaultView: vi.fn(), loadView: mocks.loadView, getDefaultView: mocks.getDefaultView,
    }),
}));
vi.mock('~/utils/search', async () => {
    const actual: any = await vi.importActual('~/utils/search');
    return { ...actual, createFilter: (...a: any[]) => { mocks.createFilter(...a); return actual.createFilter(...a); } };
});
vi.mock('./SearchSelect', () => ({ default: () => null }));
vi.mock('./BulkActionBar', () => ({ default: mocks.BulkActionBar }));
vi.mock('./ColumnConfigurator', () => ({ default: () => null }));
vi.mock('./RefreshControls', () => ({ default: () => null }));
vi.mock('./SavedViewsDropdown', () => ({ default: () => null }));
vi.mock('./ResourceLoadingStatus', () => ({ default: () => null }));
vi.mock('./ResourceListControls', () => ({
    SearchHighlightOverlay: () => null, TriStateCheckbox: () => null, RowCheckbox: () => null, SortableHeader: () => null,
}));

import ResourceList from './ResourceList';
import ResourceListHeader from './ResourceListHeader';
import { RowCells, SkeletonCells } from './ResourceListRows';
import { refItem, type RowItem } from '~/lib/views/rowItem';
import type { ViewSnapshot } from '~/lib/views/viewClient';

// ---- a fake view handle ---------------------------------------------------------------------

function row(i: number): RowItem {
    return {
        metadata: { uid: `u${i}`, name: `cm-${i}`, namespace: 'ns' },
        cells: { name: `cm-${i}`, namespace: 'ns', keys: 'a, b', age: '2024-01-01T00:00:00Z' },
        __seq: i + 1,
        __view: true,
    };
}

function fakeSource(opts: { filtered?: number; total?: number; loaded?: number[]; filterable?: Record<string, boolean> } = {}) {
    const loaded = new Map<number, RowItem>((opts.loaded ?? [0, 1, 2]).map(i => [i, row(i)]));
    let snapshot: ViewSnapshot = {
        status: 'open', error: null, permanent: false,
        state: { version: 5, total: opts.total ?? 1000, filtered: opts.filtered ?? 1000, phase: 'complete', loaded: 0, expected: 0, filterable: opts.filterable ?? { keys: true, namespace: true } },
        columns: ['name', 'namespace', 'keys', 'age'], filterable: opts.filterable ?? { keys: true, namespace: true }, schemaVersion: 's', schema: null, epoch: 1, closedReason: null,
        total: opts.total ?? 1000, filtered: opts.filtered ?? 1000,
    };
    const listeners = new Set<() => void>();
    const src = {
        viewId: 'v1',
        open: vi.fn(),
        update: vi.fn(),
        close: vi.fn(),
        getSnapshot: () => snapshot,
        subscribe: (cb: () => void) => { listeners.add(cb); return () => { listeners.delete(cb); }; },
        setRange: vi.fn(),
        rowAt: (i: number) => loaded.get(i),
        loadedRows: () => [...loaded.entries()].sort((a, b) => a[0] - b[0]).map(e => e[1]),
        refs: vi.fn(async (from: number, to: number) => Array.from({ length: to - from + 1 }, (_, k) => ({ uid: `u${from + k}`, ns: 'ns', name: `cm-${from + k}` }))),
        selectAllRefs: vi.fn(async () => Array.from({ length: 4 }, (_, k) => ({ uid: `u${k}`, ns: 'ns', name: `cm-${k}` }))),
        facet: vi.fn(async (column: string) => ({ column, values: ['a, b', 'z'], counts: { 'a, b': 3, z: 1 }, suppressed: false })),
        setSelection: vi.fn(),
        onSelectionStale: vi.fn((cb: (u: string[]) => void) => { src.staleCb = cb; return () => {}; }),
        getObject: vi.fn(),
        getObjects: vi.fn(),
        refresh: vi.fn(),
        reopen: vi.fn(),
        setActive: vi.fn(),
        spec: () => null,
        staleCb: null as null | ((u: string[]) => void),
        set(next: Partial<ViewSnapshot>) { snapshot = { ...snapshot, ...next, epoch: snapshot.epoch + 1 }; listeners.forEach(l => l()); },
    };
    return src;
}

const columns: any[] = [
    { key: 'name', label: 'Name', render: (i: RowItem) => i.cells.name, getValue: (i: RowItem) => i.cells.name },
    { key: 'namespace', label: 'Namespace', render: (i: RowItem) => i.cells.namespace, getValue: (i: RowItem) => i.cells.namespace },
    { key: 'keys', label: 'Keys', render: (i: RowItem) => i.cells.keys, getValue: (i: RowItem) => i.cells.keys },
    { key: 'age', label: 'Age', render: (i: RowItem) => i.cells.age, getValue: (i: RowItem) => i.cells.age },
    { key: 'actions', label: '', render: () => null, isColumnSelector: true, disableSort: true },
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
const renderHeader = (element: any) => {
    if (element?.type !== ResourceListHeader) return element;
    const saved = runner.index;
    runner.index = 1000;
    try { return (ResourceListHeader as any).type(element.props); } finally { runner.index = saved; }
};

function makeSelection() {
    const state = { ids: new Set<string>(), refs: new Map<string, any>(), last: null as number | null };
    const sel: any = {
        get selectedIds() { return state.ids; },
        get selectedCount() { return state.ids.size; },
        isSelected: (uid: string) => state.ids.has(uid),
        getSelectionState: vi.fn(() => 'none'),
        toggleAll: vi.fn(),
        toggleItem: vi.fn(),
        getSelectedItems: vi.fn(() => []),
        deselectAll: vi.fn(() => { state.ids = new Set(); state.refs.clear(); }),
        pruneSelection: vi.fn(),
        toggleRef: vi.fn((ref: any, index: number) => { state.ids = new Set(state.ids).add(ref.metadata.uid); state.refs.set(ref.metadata.uid, ref); state.last = index; }),
        addRefs: vi.fn((refs: any[], index?: number) => { const n = new Set(state.ids); refs.forEach(r => { n.add(r.metadata.uid); state.refs.set(r.metadata.uid, r); }); state.ids = n; if (index !== undefined) state.last = index; }),
        setRefs: vi.fn((refs: any[]) => { state.ids = new Set(refs.map(r => r.metadata.uid)); refs.forEach(r => state.refs.set(r.metadata.uid, r)); }),
        getRefItems: vi.fn(() => [...state.refs.values()]),
        getLastIndex: () => state.last,
        pruneByUids: vi.fn(),
    };
    return sel;
}

function mount(props: Record<string, any> = {}) {
    const baseProps: Record<string, any> = {
        title: 'ConfigMaps', columns, data: [], isLoading: false, resourceType: 'configmaps', namespaces: [], currentNamespace: '*',
        selectable: true, onBulkDelete: vi.fn(), ...props,
    };
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
    const header = (key: string) => findAll(renderHeader(table()!.props.fixedHeaderContent()), el => el.props?.id === key && el.props?.className?.includes('cursor-pointer'))[0];
    return {
        render, table, tree: () => tree, props: baseProps,
        setProps: (next: Record<string, any>) => { Object.assign(baseProps, next); render(); },
        clickHeader: (key: string) => {
            const label = findAll(header(key), el => el.type === 'div' && typeof el.props.onClick === 'function')[0];
            label.props.onClick();
            render();
        },
        openFilter: (key: string) => {
            findAll(header(key), el => el.type === 'button')[0].props.onClick({
                stopPropagation() {},
                currentTarget: { getBoundingClientRect: () => ({ left: 10, top: 10, bottom: 30 }) },
            });
            render();
        },
        hasFilterButton: (key: string) => Boolean(header(key)) && findAll(header(key), el => el.type === 'button').length > 0,
        headerCheckbox: () => renderHeader(table()!.props.fixedHeaderContent()),
        searchInput: () => findAll(tree, el => el.type === 'input' && String(el.props.placeholder).startsWith('Search'))[0],
        dropdown: () => findAll(tree, el => el.type === 'label').map(label => {
            const spans = findAll(label, el => el.type === 'span');
            return { value: String(spans[0].props.children), count: Number(spans[1].props.children) };
        }),
        item: (index: number) => table().props.itemContent(index),
    };
}

const flush = async () => { for (let i = 0; i < 10; i++) await Promise.resolve(); };

beforeEach(() => {
    runner.slots.forEach(slot => slot?.cleanup?.());
    runner.slots = []; runner.index = 0; runner.effects = []; runner.dirty = false;
    mocks.savedView.current = null;
    mocks.createFilter.mockReset();
    vi.useFakeTimers();
    const g = globalThis as any;
    g.localStorage = { getItem: () => null, setItem: () => {}, removeItem: () => {} };
    g.window = { innerWidth: 1200, innerHeight: 800 };
    g.document = { body: {}, hidden: false, addEventListener: () => {}, removeEventListener: () => {} };
});
afterEach(() => {
    runner.slots.forEach(slot => slot?.cleanup?.());
    vi.useRealTimers();
});

describe('ResourceList view mode rendering', () => {
    it('renders the table by count, not by data, and builds rows by index', () => {
        const source = fakeSource({ filtered: 1000, loaded: [0, 1, 2] });
        const view = mount({ source });
        const table = view.table();
        expect(table.props.totalCount).toBe(1000);
        expect(table.props.data).toBeUndefined();
        const loaded = view.item(1);
        expect(loaded.type).toBe(RowCells);
        expect(loaded.props.item).toBe(source.rowAt(1));
        expect(loaded.props.index).toBe(1);
    });

    it('shows skeleton rows for indices whose window is still in flight', () => {
        const source = fakeSource({ loaded: [0, 1] });
        const view = mount({ source });
        const placeholder = view.item(500);
        expect(placeholder.type).toBe(SkeletonCells);
        expect(table(view).props.computeItemKey(500)).toBe('pending-500');
        expect(table(view).props.computeItemKey(1)).toBe('u1');
    });

    it('reports the visible range to the view with an exclusive end', () => {
        const source = fakeSource();
        const view = mount({ source });
        view.table().props.rangeChanged({ startIndex: 10, endIndex: 24 });
        expect(source.setRange).toHaveBeenCalledWith(10, 25);
    });

    it('re-renders when the view publishes a new window', () => {
        const source = fakeSource({ filtered: 3, loaded: [0] });
        const view = mount({ source });
        expect(view.item(1).type).toBe(SkeletonCells);
        (source as any).rowAt = (i: number) => row(i);
        source.set({ filtered: 3 });
        view.render();
        expect(view.item(1).type).toBe(RowCells);
    });

    it('follows the view counts for the table size', () => {
        const source = fakeSource({ filtered: 10, total: 10 });
        const view = mount({ source });
        source.set({ filtered: 4, total: 10 });
        view.render();
        expect(view.table().props.totalCount).toBe(4);
    });

    it('does not run the in-component search pipeline', () => {
        const source = fakeSource();
        const view = mount({ source, data: [row(1), row(2)] });
        view.searchInput().props.onChange({ target: { value: 'name:x' } });
        view.render();
        vi.advanceTimersByTime(10);
        view.render();
        expect(mocks.createFilter).not.toHaveBeenCalled();
    });

    it('shows loading while the view has no rows yet and the empty state once it is complete', () => {
        const loading = fakeSource({ filtered: 0, total: 0 });
        const view = mount({ source: loading, isLoading: true });
        expect(view.table()).toBeUndefined();
        const empty = fakeSource({ filtered: 0, total: 0 });
        const view2 = mount({ source: empty, isLoading: false });
        expect(view2.table()).toBeUndefined();
    });

    it('uses the legacy path when there is no source', () => {
        const view = mount({ source: null, data: [row(1)] });
        expect(view.table().props.data).toHaveLength(1);
        expect(view.table().props.totalCount).toBeUndefined();
    });
});

describe('ResourceList view mode layout', () => {
    it('reserves the scrollbar gutter so columns do not shift when the bar comes and goes', () => {
        const view = mount({ source: fakeSource({ filtered: 3, loaded: [0, 1, 2] }) });
        const style = view.table().props.style;
        expect(style.scrollbarGutter).toBe('stable');
        expect(style.overflowY).toBe('scroll');
        expect(style.height).toBe('100%');
    });

    it('leaves the legacy table scroller as it was', () => {
        const view = mount({ source: null, data: [row(1)] });
        expect(view.table().props.style).toEqual({ height: '100%' });
    });
});

describe('ResourceList selection bar label', () => {
    const barProps = (view: any) => findAll(view.tree(), el => el.type === mocks.BulkActionBar)[0].props;

    it('never shows the internal custom resource id', () => {
        const selection = makeSelection();
        selection.toggleRef(refItem('u1', 'ns', 'r1'), 1);
        const view = mount({
            source: fakeSource(), selection, title: 'PrometheusRule',
            resourceType: 'cr-monitoring.coreos.com-prometheusrules', selectionLabel: 'prometheusrules',
        });
        expect(barProps(view).resourceType).toBe('prometheusrules');
    });

    it('falls back to the title when a custom resource list passes no label', () => {
        const selection = makeSelection();
        selection.toggleRef(refItem('u1', 'ns', 'r1'), 1);
        const view = mount({ source: fakeSource(), selection, title: 'PrometheusRule', resourceType: 'cr-monitoring.coreos.com-prometheusrules' });
        expect(barProps(view).resourceType).toBe('prometheusrule');
    });

    it('keeps the resource type for built-in lists', () => {
        const selection = makeSelection();
        selection.toggleRef(refItem('u1', 'ns', 'r1'), 1);
        const view = mount({ source: fakeSource(), selection });
        expect(barProps(view).resourceType).toBe('configmaps');
    });
});

describe('ResourceList view mode queries', () => {
    it('pushes the initial sort to the view and then every sort change at once', () => {
        const source = fakeSource();
        const view = mount({ source, initialSort: { key: 'age', direction: 'desc' } });
        expect(source.update).toHaveBeenCalledWith(expect.objectContaining({ sort: { key: 'age', dir: 'desc' } }));
        source.update.mockClear();
        view.clickHeader('name');
        expect(source.update).toHaveBeenLastCalledWith(expect.objectContaining({ sort: { key: 'name', dir: 'asc' } }));
        view.clickHeader('name');
        expect(source.update).toHaveBeenLastCalledWith(expect.objectContaining({ sort: { key: 'name', dir: 'desc' } }));
    });

    it('sends search text after the debounce, not per keystroke', () => {
        const source = fakeSource();
        const view = mount({ source });
        source.update.mockClear();
        view.searchInput().props.onChange({ target: { value: 'ngi' } });
        view.render();
        expect(source.update.mock.calls.some(c => 'search' in c[0] && c[0].search === 'ngi')).toBe(false);
        vi.advanceTimersByTime(10);
        view.render();
        expect(source.update).toHaveBeenCalledWith({ search: 'ngi' });
    });

    it('sends column filters with Sets as arrays', () => {
        mocks.savedView.current = {
            id: 'sv', query: '', columnFilters: { keys: { type: 'select', values: ['a, b'] }, name: { type: 'regex', conditions: ['^cm'], logic: 'and' } },
        };
        const source = fakeSource();
        mount({ source });
        const call = source.update.mock.calls.map(c => c[0]).find(c => c.columnFilters?.length);
        expect(call.columnFilters).toEqual([
            { col: 'keys', type: 'select', values: ['a, b'] },
            { col: 'name', type: 'regex', conditions: ['^cm'], logic: 'and' },
        ]);
    });

    it('projects only the visible schema columns', () => {
        const source = fakeSource();
        const view = mount({ source, columns: [...columns.slice(0, 3), { ...columns[3], defaultHidden: true }, columns[4]] });
        expect(source.update).toHaveBeenCalledWith({ columns: ['name', 'namespace', 'keys'] });
        void view;
    });

    it('a new view receives the whole query again', () => {
        const first = fakeSource();
        const view = mount({ source: first, initialSort: { key: 'age', direction: 'desc' } });
        const second = fakeSource();
        view.setProps({ source: second });
        expect(second.update).toHaveBeenCalledWith(expect.objectContaining({ sort: { key: 'age', dir: 'desc' } }));
        expect(second.update).toHaveBeenCalledWith({ columns: ['name', 'namespace', 'keys', 'age'] });
    });

    it('never pushes a sort on a column the schema does not know', () => {
        const source = fakeSource();
        mount({ source, columns: [...columns, { key: 'cpu', label: 'CPU', render: () => null }], initialSort: { key: 'cpu', direction: 'asc' } });
        expect(source.update).toHaveBeenCalledWith(expect.objectContaining({ sort: { key: '', dir: '' } }));
    });
});

describe('ResourceList view mode filters', () => {
    it('takes filter types and funnel visibility from the schema and the view', () => {
        const source = fakeSource({ filterable: { keys: true, namespace: false } });
        const view = mount({ source });
        expect(view.hasFilterButton('name')).toBe(true); // regex
        expect(view.hasFilterButton('age')).toBe(true); // numeric
        expect(view.hasFilterButton('keys')).toBe(true); // select with values
        const nothing = fakeSource({ filterable: { keys: false } });
        expect(mount({ source: nothing }).hasFilterButton('keys')).toBe(false);
    });

    it('loads the select values from the view when a dropdown opens', async () => {
        const source = fakeSource();
        const view = mount({ source });
        view.openFilter('keys');
        vi.advanceTimersByTime(1);
        await flush();
        view.render();
        expect(source.facet).toHaveBeenCalledWith('keys');
        expect(view.dropdown()).toEqual([{ value: 'a, b', count: 3 }, { value: 'z', count: 1 }]);
    });

    it('does not ask for values until a dropdown opens', () => {
        const source = fakeSource();
        mount({ source });
        expect(source.facet).not.toHaveBeenCalled();
    });
});

describe('ResourceList view mode selection', () => {
    it('select-all fetches every filtered ref instead of reading rows', async () => {
        const source = fakeSource();
        const selection = makeSelection();
        const view = mount({ source, selection });
        // The header's checkbox handler is the component's own callback.
        const handler = (renderHeaderProps(view)).onHeaderCheckboxClick;
        handler();
        await flush();
        expect(source.selectAllRefs).toHaveBeenCalledTimes(1);
        expect(selection.setRefs).toHaveBeenCalledWith(expect.arrayContaining([
            { metadata: { uid: 'u0', name: 'cm-0', namespace: 'ns' } },
        ]));
        expect(selection.selectedCount).toBe(4);
        expect(selection.toggleAll).not.toHaveBeenCalled();
    });

    it('select-all clears when something is already selected', () => {
        const source = fakeSource();
        const selection = makeSelection();
        selection.toggleRef(refItem('u1', 'ns', 'cm-1'), 1);
        const view = mount({ source, selection });
        renderHeaderProps(view).onHeaderCheckboxClick();
        expect(selection.deselectAll).toHaveBeenCalled();
        expect(source.selectAllRefs).not.toHaveBeenCalled();
    });

    it('a plain row click toggles that row by identity', () => {
        const source = fakeSource();
        const selection = makeSelection();
        const view = mount({ source, selection });
        view.item(2).props.onToggleSelect({ shiftKey: false }, source.rowAt(2), 2);
        expect(selection.toggleRef).toHaveBeenCalledWith({ metadata: { uid: 'u2', name: 'cm-2', namespace: 'ns' } }, 2);
    });

    it('a shift click selects the range through the refs stream', async () => {
        const source = fakeSource();
        const selection = makeSelection();
        const view = mount({ source, selection });
        view.item(1).props.onToggleSelect({ shiftKey: false }, source.rowAt(1), 1);
        view.render();
        view.item(2).props.onToggleSelect({ shiftKey: true }, source.rowAt(2), 2);
        await flush();
        expect(source.refs).toHaveBeenCalledWith(1, 2);
        expect(selection.addRefs).toHaveBeenCalledWith(expect.any(Array), 2);
        expect([...selection.selectedIds].sort()).toEqual(['u1', 'u2']);
    });

    it('bulk actions receive identity items from the selection, never rows read from the table', () => {
        const source = fakeSource();
        const selection = makeSelection();
        selection.setRefs([refItem('u1', 'ns', 'cm-1'), refItem('u900', 'ns', 'cm-900')]);
        const onBulkDelete = vi.fn();
        const view = mount({ source, selection, onBulkDelete });
        const bar = findAll(view.tree(), el => el.props?.customActions !== undefined && typeof el.props?.onClearSelection === 'function')[0];
        bar.props.onDelete();
        expect(onBulkDelete).toHaveBeenCalledWith([
            { metadata: { uid: 'u1', name: 'cm-1', namespace: 'ns' } },
            { metadata: { uid: 'u900', name: 'cm-900', namespace: 'ns' } },
        ]);
        expect(selection.getSelectedItems).not.toHaveBeenCalled();
    });

    it('tells the view which uids are selected and prunes the ones it reports stale', () => {
        const source = fakeSource();
        const selection = makeSelection();
        selection.setRefs([refItem('u1', 'ns', 'cm-1')]);
        mount({ source, selection });
        expect(source.setSelection).toHaveBeenCalledWith(selection.selectedIds);
        source.staleCb!(['u1']);
        expect(selection.pruneByUids).toHaveBeenCalledWith(['u1']);
        expect(selection.pruneSelection).not.toHaveBeenCalled();
    });

    it('reports the header state from counts', () => {
        const source = fakeSource({ filtered: 4 });
        const selection = makeSelection();
        selection.setRefs([refItem('u1', 'ns', 'cm-1')]);
        const view = mount({ source, selection });
        expect(renderHeaderProps(view).selectionState).toBe('some');
        const everything = makeSelection();
        everything.setRefs(Array.from({ length: 4 }, (_, i) => refItem(`u${i}`, 'ns', `cm-${i}`)));
        view.setProps({ selection: everything });
        expect(renderHeaderProps(view).selectionState).toBe('all');
    });
});

describe('ResourceList view mode consumers', () => {
    it('fetches the filtered uids lazily for a consumer that registered', async () => {
        const source = fakeSource();
        const onFilteredUidsChange = vi.fn();
        mount({ source, onFilteredUidsChange });
        expect(source.selectAllRefs).not.toHaveBeenCalled();
        vi.advanceTimersByTime(400);
        await flush();
        expect(onFilteredUidsChange).toHaveBeenCalledWith(new Set(['u0', 'u1', 'u2', 'u3']));
    });
});

// ---- helpers --------------------------------------------------------------------------------

function table(view: ReturnType<typeof mount>) {
    return view.table();
}

function renderHeaderProps(view: ReturnType<typeof mount>): any {
    return view.table().props.fixedHeaderContent().props;
}
