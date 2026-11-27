import { WorkerCore } from './viewWorkerCore';
import type { ToWorker } from './workerProtocol';

// Entry of the view worker. All behaviour lives in WorkerCore; this file only binds it to the
// worker global scope. It must not import anything that needs the Wails runtime.
const scope = self as unknown as {
    postMessage(msg: unknown, transfer?: Transferable[]): void;
    onmessage: ((ev: MessageEvent<ToWorker>) => void) | null;
};

const core = new WorkerCore((msg, transfer) => scope.postMessage(msg, transfer ?? []));
scope.onmessage = (ev) => core.handle(ev.data);
