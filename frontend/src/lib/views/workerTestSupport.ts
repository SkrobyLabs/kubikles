import { ViewGoneError } from './protocol';
import { FakeBackend } from './testSupport';
import type { HostServices, WorkerLike } from './viewHost';
import { WorkerCore, type WorkerCoreDeps } from './viewWorkerCore';
import type { FromWorker, ToWorker } from './workerProtocol';

// An in-memory worker: messages cross a structured-clone boundary (so anything not clonable fails
// the test) and are delivered in order on microtasks, like a real worker channel.

export interface FakeWorker extends WorkerLike {
    core: WorkerCore;
    /** Messages the worker sent, in order. */
    sent: FromWorker[];
    /** Messages the main thread sent, in order. */
    received: ToWorker[];
    /** The worker stops answering and stops sending heartbeats. */
    hang(): void;
    crash(message?: string): void;
    terminated: boolean;
}

export function makeFakeWorker(coreDeps: WorkerCoreDeps = {}): FakeWorker {
    let hung = false;
    const w: FakeWorker = {
        core: null as unknown as WorkerCore,
        sent: [],
        received: [],
        terminated: false,
        onmessage: null,
        onerror: null,
        postMessage(msg: unknown, transfer?: Transferable[]) {
            if (w.terminated || hung) return;
            const data = structuredClone(msg, { transfer: transfer ?? [] }) as ToWorker;
            w.received.push(data);
            queueMicrotask(() => { if (!w.terminated && !hung) w.core.handle(data); });
        },
        terminate() {
            w.terminated = true;
            w.core.dispose();
        },
        hang() { hung = true; },
        crash(message = 'boom') { w.onerror?.({ message }); },
    };
    w.core = new WorkerCore((msg, transfer) => {
        if (w.terminated || hung) return;
        const data = structuredClone(msg, { transfer: transfer ?? [] }) as FromWorker;
        w.sent.push(data);
        queueMicrotask(() => { if (!w.terminated) w.onmessage?.({ data }); });
    }, coreDeps);
    return w;
}

/** A fetch that serves the view routes from a FakeBackend. */
export function backendFetch(backend: FakeBackend, log: string[] = []): typeof fetch {
    return (async (input: RequestInfo | URL) => {
        const url = new URL(String(input));
        log.push(url.pathname + url.search);
        const parts = url.pathname.split('/').filter(Boolean);
        const route = parts.slice(2);
        try {
            if (route[0] === 'views' && route[2] === 'window') {
                const q = url.searchParams;
                const win = await backend.window(route[1], Number(q.get('offset')), Number(q.get('limit')), Number(q.get('have')));
                return new Response(JSON.stringify(win), { status: 200, headers: { 'Content-Type': 'application/json' } });
            }
            if (route[0] === 'views' && route[2] === 'refs') {
                const q = url.searchParams;
                const refs = await backend.refs(route[1], Number(q.get('offset')), Number(q.get('limit')));
                const lines = [JSON.stringify({ version: 1, total: refs.length, filtered: refs.length })];
                for (const r of refs) lines.push(JSON.stringify([r.uid, r.ns, r.name]));
                return new Response(lines.join('\n') + '\n', { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } });
            }
            if (route[0] === 'objects') {
                return new Response(JSON.stringify({ metadata: { name: url.searchParams.get('name') }, via: 'fetch' }), {
                    status: 200, headers: { 'Content-Type': 'application/json' },
                });
            }
            if (route[0] === 'call') {
                return new Response(JSON.stringify({ data: `called ${route[1]}` }), { status: 200, headers: { 'Content-Type': 'application/json' } });
            }
        } catch (e) {
            if (e instanceof ViewGoneError) {
                return new Response(JSON.stringify({ error: 'view not found' }), { status: 410, headers: { 'Content-Type': 'application/json' } });
            }
            return new Response(JSON.stringify({ error: (e as Error).message }), { status: 500, headers: { 'Content-Type': 'application/json' } });
        }
        return new Response('{}', { status: 404 });
    }) as typeof fetch;
}

export function backendServices(backend: FakeBackend): HostServices {
    return {
        transport: backend,
        fetchBytes: async (path: string) => {
            const f = backendFetch(backend);
            const r = await f(`http://localhost/_kubikles/v1/${path}`);
            return { status: r.status, contentType: r.headers.get('Content-Type') ?? '', bytes: await r.arrayBuffer() };
        },
    };
}
