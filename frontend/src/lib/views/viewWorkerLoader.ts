import ViewWorker from './viewWorker?worker&inline';
import type { WorkerLike } from './viewHost';

/**
 * Builds the view worker from an inline Blob so loading it never depends on the asset scheme
 * handler. Kept in its own module so the worker bundle is only pulled in when a worker is wanted.
 */
export function createViewWorker(): WorkerLike {
    return new ViewWorker() as unknown as WorkerLike;
}
