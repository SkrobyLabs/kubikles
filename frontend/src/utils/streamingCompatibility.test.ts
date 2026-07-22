import { describe, expect, it } from 'vitest';
import { isImmediateWatchClosure, isStreamTransportError, isStreamingWarningDismissed, restoreConnectionMode } from './streamingCompatibility';

describe('streaming compatibility detection', () => {
    it('detects the Paralus HTTP-200-then-close signal without waiting for two errors', () => {
        expect(isImmediateWatchClosure({ premature: true, receivedAny: false })).toBe(true);
        expect(isImmediateWatchClosure({ premature: true, receivedAny: true })).toBe(false);
    });

    it('recognizes common proxy stream failures', () => {
        expect(isStreamTransportError('unexpected EOF')).toBe(true);
        expect(isStreamTransportError('connection reset by peer')).toBe(true);
        expect(isStreamTransportError('Forbidden')).toBe(false);
    });

    it('restores modes independently for each context', () => {
        const values = new Map([['kubikles_connection_mode_paralus', 'polling']]);
        const storage = { getItem: (key: string) => values.get(key) ?? null };
        expect(restoreConnectionMode('paralus', storage)).toBe('polling');
        expect(restoreConnectionMode('direct', storage)).toBe('streaming');
    });

    it('persists prompt dismissal independently for each context', () => {
        const values = new Map([['kubikles_streaming_warning_dismissed_paralus', 'true']]);
        const storage = { getItem: (key: string) => values.get(key) ?? null };
        expect(isStreamingWarningDismissed('paralus', storage)).toBe(true);
        expect(isStreamingWarningDismissed('direct', storage)).toBe(false);
    });
});
