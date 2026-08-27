export interface ContextSwitchTransaction {
    currentContext: string;
    nextContext: string;
    switchBackend: (context: string) => Promise<void>;
    setPending: (pending: boolean) => void;
    commit: () => void;
}

// Runs the fallible backend part of a context switch before mutating active
// context-dependent state. A rejected switch only clears the pending indicator.
export const runContextSwitchTransaction = async ({
    currentContext,
    nextContext,
    switchBackend,
    setPending,
    commit,
}: ContextSwitchTransaction): Promise<boolean> => {
    if (currentContext === nextContext) {
        return false;
    }

    setPending(true);
    try {
        await switchBackend(nextContext);
        commit();
        return true;
    } finally {
        setPending(false);
    }
};
