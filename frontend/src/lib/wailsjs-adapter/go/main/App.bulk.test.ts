import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// The adapter routes bulk-eligible reads through the fetch route and leaves
// every other method on the binding / HTTP API path. Wails bindings are faked
// on window.go; fetch is faked globally.

const binding = {
    GetBulkInfo: vi.fn(),
    ListPods: vi.fn(),
    GetPodYaml: vi.fn(),
    DeletePod: vi.fn(),
    SetForceHTTP1: vi.fn(),
};
let fetchMock: ReturnType<typeof vi.fn>;
let storage: Record<string, string>;

const capsOk = () => ({
    enabled: true, fetch: true, streaming: true, buffered: false, workerBlob: false, workerFetch: false,
    workerScriptUrl: false, degraded: false, notes: [], probedAt: 1, userAgent: 't',
    benchMs: { bulkJson: null, bulkNdjson: null, ipc: null },
});

beforeEach(() => {
    vi.resetModules();
    for (const f of Object.values(binding)) f.mockReset();
    binding.GetBulkInfo.mockResolvedValue({ enabled: true, base: '/_kubikles/v1', token: 'tok', streaming: true, server: false, version: 1 });
    binding.ListPods.mockResolvedValue([{ via: 'binding' }]);
    binding.GetPodYaml.mockResolvedValue('binding yaml');
    binding.DeletePod.mockResolvedValue(undefined);
    binding.SetForceHTTP1.mockResolvedValue(undefined);
    (globalThis as any).window = { go: { main: { App: binding } } };
    fetchMock = vi.fn();
    (globalThis as any).fetch = fetchMock;
    storage = {};
    (globalThis as any).localStorage = {
        getItem: (k: string) => (k in storage ? storage[k] : null),
        setItem: (k: string, v: string) => { storage[k] = v; },
        removeItem: (k: string) => { delete storage[k]; },
    };
});

afterEach(() => {
    delete (globalThis as any).window;
    delete (globalThis as any).fetch;
    delete (globalThis as any).localStorage;
});

async function load() {
    const transport = await import('../../../bulk/transport');
    transport.resetBulkCaps();
    const app = await import('./App');
    return { app, transport };
}

describe('wails adapter bulk routing', () => {
    it('uses the binding for everything until the probe has passed on desktop', async () => {
        const { app } = await load();
        expect(await app.ListPods('r', '')).toEqual([{ via: 'binding' }]);
        expect(fetchMock).not.toHaveBeenCalled();
    });

    it('routes list and yaml reads through fetch once caps allow it', async () => {
        const { app, transport } = await load();
        await transport.getBulkCaps();
        transport.applyProbedCaps(capsOk() as any);
        fetchMock.mockResolvedValueOnce(new Response('{"via":"bulk"}\n', { headers: { 'Content-Type': 'application/x-ndjson' } }));
        expect(await app.ListPods('r', 'ns')).toEqual([{ via: 'bulk' }]);
        fetchMock.mockResolvedValueOnce(new Response('{"data":"bulk yaml"}', { headers: { 'Content-Type': 'application/json' } }));
        expect(await app.GetPodYaml('ns', 'p')).toBe('bulk yaml');
        expect(binding.ListPods).not.toHaveBeenCalled();
        expect(binding.GetPodYaml).not.toHaveBeenCalled();
    });

    it('falls back to the binding for that call when fetch fails', async () => {
        const { app, transport } = await load();
        await transport.getBulkCaps();
        transport.applyProbedCaps(capsOk() as any);
        fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'));
        expect(await app.ListPods('r', 'ns')).toEqual([{ via: 'binding' }]);
        expect(binding.ListPods).toHaveBeenCalledWith('r', 'ns');
    });

    it('never routes non-bulk methods through fetch', async () => {
        const { app, transport } = await load();
        await transport.getBulkCaps();
        transport.applyProbedCaps(capsOk() as any);
        await app.DeletePod('ns', 'p');
        await app.SetForceHTTP1(true);
        expect(fetchMock).not.toHaveBeenCalled();
        expect(binding.DeletePod).toHaveBeenCalledWith('ns', 'p');
        expect(binding.SetForceHTTP1).toHaveBeenCalledWith(true);
    });

    it('a method error over bulk is not retried on the binding', async () => {
        const { app, transport } = await load();
        await transport.getBulkCaps();
        transport.applyProbedCaps(capsOk() as any);
        fetchMock.mockResolvedValueOnce(new Response('{"error":"forbidden"}', { status: 500, headers: { 'Content-Type': 'application/json' } }));
        await expect(app.ListPods('r', '')).rejects.toBe('forbidden');
        expect(binding.ListPods).not.toHaveBeenCalled();
    });
});

