export function createPollingBudget(maxConcurrent: number) {
    let active = 0;
    const waiting: Array<() => void> = [];
    const acquire = async () => {
        if (active < maxConcurrent) {
            active += 1;
            return;
        }
        await new Promise<void>(resolve => waiting.push(resolve));
        active += 1;
    };
    const release = () => {
        active -= 1;
        waiting.shift()?.();
    };
    return async <T>(operation: () => Promise<T>): Promise<T> => {
        await acquire();
        try {
            return await operation();
        } finally {
            release();
        }
    };
}

export const runWithPollingBudget = createPollingBudget(4);
