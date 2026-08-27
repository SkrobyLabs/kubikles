import { describe, expect, it, vi } from 'vitest';
import { runContextSwitchTransaction } from './contextSwitch';

describe('runContextSwitchTransaction', () => {
    it('keeps active context-dependent state intact when the backend rejects', async () => {
        const state = {
            context: 'old', ref: 'old', namespaces: ['default'], selectedNamespaces: ['default'],
            connectionError: 'previous error', lastContext: 'old',
        };
        const pending: boolean[] = [];
        const commit = vi.fn(() => {
            state.context = 'new';
            state.ref = 'new';
            state.namespaces = [];
            state.selectedNamespaces = [];
            state.connectionError = '';
            state.lastContext = 'new';
        });

        await expect(runContextSwitchTransaction({
            currentContext: state.context,
            nextContext: 'new',
            switchBackend: vi.fn().mockRejectedValue(new Error('switch failed')),
            setPending: (value) => pending.push(value),
            commit,
        })).rejects.toThrow('switch failed');

        expect(pending).toEqual([true, false]);
        expect(commit).not.toHaveBeenCalled();
        expect(state).toEqual({
            context: 'old', ref: 'old', namespaces: ['default'], selectedNamespaces: ['default'],
            connectionError: 'previous error', lastContext: 'old',
        });
    });

    it('commits exactly once after a successful backend switch', async () => {
        let resolveBackend!: () => void;
        const backend = vi.fn(() => new Promise<void>((resolve) => { resolveBackend = resolve; }));
        const commit = vi.fn();
        const pending: boolean[] = [];
        const transaction = runContextSwitchTransaction({
            currentContext: 'old', nextContext: 'new', switchBackend: backend,
            setPending: (value) => pending.push(value), commit,
        });

        expect(commit).not.toHaveBeenCalled();
        resolveBackend();
        await expect(transaction).resolves.toBe(true);
        expect(commit).toHaveBeenCalledTimes(1);
        expect(pending).toEqual([true, false]);
    });

    it('does nothing when switching to the active context', async () => {
        const backend = vi.fn();
        const pending = vi.fn();
        const commit = vi.fn();

        await expect(runContextSwitchTransaction({
            currentContext: 'same', nextContext: 'same', switchBackend: backend, setPending: pending, commit,
        })).resolves.toBe(false);

        expect(backend).not.toHaveBeenCalled();
        expect(pending).not.toHaveBeenCalled();
        expect(commit).not.toHaveBeenCalled();
    });
});
