/**
 * Utility for executing bulk operations sequentially with an optional delay between each.
 */

export function sleep(ms: number): Promise<void> {
    if (ms <= 0) return Promise.resolve();
    return new Promise(resolve => setTimeout(resolve, ms));
}

export interface BulkExecutionOptions<T> {
    delayMs?: number;
    onProgress?: (current: number, total: number, item: T, success: boolean) => void;
}

/**
 * Execute an operation on each item sequentially, with an optional delay between items.
 * Continues on failure. No delay after the last item.
 */
export async function executeBulkOperations<T>(
    items: T[],
    operation: (item: T) => Promise<void>,
    options: BulkExecutionOptions<T> = {},
): Promise<{ item: T; success: boolean; error?: unknown }[]> {
    const { delayMs = 0, onProgress } = options;
    const results: { item: T; success: boolean; error?: unknown }[] = [];

    for (let i = 0; i < items.length; i++) {
        const item = items[i];
        let success = true;
        let error: unknown;

        try {
            await operation(item);
        } catch (err) {
            success = false;
            error = err;
        }

        results.push({ item, success, error });
        onProgress?.(i + 1, items.length, item, success);

        // Delay between items, not after the last one
        if (delayMs > 0 && i < items.length - 1) {
            await sleep(delayMs);
        }
    }

    return results;
}
