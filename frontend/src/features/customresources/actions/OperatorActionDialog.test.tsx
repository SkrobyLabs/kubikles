import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ActionPlan, OperatorAction, ResourceRef } from './types';

const harness = vi.hoisted(() => ({ states: [] as any[], refs: [] as any[], effects: [] as Array<() => any>, stateIndex: 0, refIndex: 0, context: 'cluster-a' }));
const api = vi.hoisted(() => ({ PrepareResourceAction: vi.fn(), ExecuteResourceAction: vi.fn() }));
vi.mock('react', async original => ({
    ...await original<typeof import('react')>(),
    useState: (initial: any) => {
        const index = harness.stateIndex++;
        if (!(index in harness.states)) harness.states[index] = typeof initial === 'function' ? initial() : initial;
        return [harness.states[index], (value: any) => { harness.states[index] = typeof value === 'function' ? value(harness.states[index]) : value; }];
    },
    useRef: (initial: any) => harness.refs[harness.refIndex++] ||= { current: initial },
    useEffect: (effect: () => any) => harness.effects.push(effect),
}));
vi.mock('react-dom', () => ({ createPortal: (node: any) => node }));
vi.mock('~/context', () => ({ useK8s: () => ({ currentContext: harness.context }) }));
vi.mock('wailsjs/go/main/App', () => api);
import OperatorActionDialog from './OperatorActionDialog';

const source: ResourceRef = { context: 'cluster-a', group: 'test.io', version: 'v1', resource: 'clusters', namespace: 'ns', name: 'test', uid: 'source' };
const action: OperatorAction = { id: 'restart', label: 'Restart…', description: 'Restart', modes: [
    { id: 'all', label: 'Whole cluster', selectTargets: false },
    { id: 'pods', label: 'Selected pods', selectTargets: true },
] };
const preview: ActionPlan = { source, requestId: 'request', actionId: action.id, mode: 'all', summary: 'Summary', targets: [
    { ...source, resource: 'pods', name: 'pod-a', uid: 'a', kind: 'Pod', pending: false },
    { ...source, resource: 'pods', name: 'pod-b', uid: 'b', kind: 'Pod', pending: false },
] };
function render() {
    harness.stateIndex = 0; harness.refIndex = 0; harness.effects = [];
    return OperatorActionDialog({ action, source, onClose: vi.fn() });
}
function nodes(node: any): any[] {
    if (!node || typeof node !== 'object') return [];
    if (Array.isArray(node)) return node.flatMap(nodes);
    return [node, ...nodes(node.props?.children)];
}
function button(tree: any, label: string) { return nodes(tree).find(node => node.type === 'button' && node.props.children === label)!; }
async function loadPreview() {
    render(); const cleanup = harness.effects[0]();
    await Promise.resolve(); await Promise.resolve(); await Promise.resolve();
    return cleanup;
}
beforeEach(() => {
    vi.clearAllMocks(); harness.states = []; harness.refs = []; harness.context = 'cluster-a';
    vi.stubGlobal('document', { body: {} });
    api.PrepareResourceAction.mockResolvedValue(preview);
    api.ExecuteResourceAction.mockResolvedValue(preview.targets.map(target => ({ target, status: 'requested' })));
});

describe('operator action confirmation', () => {
    it('requires a completed preview and blocks a switched context', async () => {
        expect(button(render(), 'Confirm request').props.disabled).toBe(true);
        await loadPreview();
        harness.context = 'cluster-b';
        const confirm = button(render(), 'Confirm request');
        expect(confirm.props.disabled).toBe(true);
        await confirm.props.onClick();
        expect(api.ExecuteResourceAction).not.toHaveBeenCalled();
    });
    it('executes only explicitly selected pods', async () => {
        await loadPreview();
        nodes(render()).find(node => node.type === 'input' && node.props.type === 'radio' && !node.props.checked)!.props.onChange();
        expect(button(render(), 'Confirm request').props.disabled).toBe(true);
        api.PrepareResourceAction.mockResolvedValue({ ...preview, mode: 'pods' });
        await loadPreview();
        expect(button(render(), 'Confirm request').props.disabled).toBe(true);
        nodes(render()).find(node => node.type === 'input' && node.props.type === 'checkbox')!.props.onChange({ target: { checked: true } });
        await button(render(), 'Confirm request').props.onClick();
        expect(api.ExecuteResourceAction).toHaveBeenCalledWith({ ...preview, mode: 'pods', targets: [preview.targets[0]] });
    });
    it('prevents duplicate submissions and displays per-target outcomes', async () => {
        await loadPreview();
        let resolve!: (result: any) => void;
        api.ExecuteResourceAction.mockReturnValue(new Promise(done => { resolve = done; }));
        const confirm = button(render(), 'Confirm request');
        const pending = confirm.props.onClick();
        await confirm.props.onClick();
        expect(api.ExecuteResourceAction).toHaveBeenCalledTimes(1);
        resolve([{ target: preview.targets[0], status: 'requested' }, { target: preview.targets[1], status: 'failed', error: 'forbidden' }]);
        await pending;
        const content = JSON.stringify(render());
        expect(content).toContain('Request accepted');
        expect(content).toContain('forbidden');
        expect(button(render(), 'Confirm request')).toBeUndefined();
    });
    it('requires a new preview after an uncertain execution failure', async () => {
        await loadPreview(); api.ExecuteResourceAction.mockRejectedValue(new Error('timeout'));
        await button(render(), 'Confirm request').props.onClick();
        expect(button(render(), 'Confirm request').props.disabled).toBe(true);
        expect(JSON.stringify(render())).toContain('may already have been accepted');
    });
    it('ignores a preview that arrives after the scope was changed', async () => {
        let resolve!: (result: any) => void;
        api.PrepareResourceAction.mockReturnValue(new Promise(done => { resolve = done; }));
        render(); const cleanup = harness.effects[0](); cleanup();
        resolve(preview); await Promise.resolve(); await Promise.resolve();
        expect(button(render(), 'Confirm request').props.disabled).toBe(true);
    });
});
