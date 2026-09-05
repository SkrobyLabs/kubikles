import { describe, expect, it } from 'vitest';
import { isImmediateWatchClosure, isStreamTransportError, restoreConnectionMode, restorePollingInterval, WatchFailureWindow } from './streamingCompatibility';

describe('watch fallback', () => {
    it('requires five rapid failures of the same watch despite successful HTTP opens', () => {
        const window = new WatchFailureWindow();
        [0, 2000, 6000, 14000, 30000].forEach((time, index) => {
            window.status('pods/default', 'connected', time);
            expect(window.fail('pods/default', time + 100)).toBe(index === 4);
            window.status('pods/default', 'reconnecting', time + 100);
        });
    });
    it('does not accumulate failures over a long session or across watches', () => {
        const window = new WatchFailureWindow();
        for (let i = 0; i < 20; i++) {
            expect(window.fail('pods', i * 60_000)).toBe(false);
            expect(window.fail('resource-' + i, i)).toBe(false);
        }
    });
    it('expires old failures at the window boundary', () => {
        const window = new WatchFailureWindow();
        [0, 1, 2, 3].forEach(time => expect(window.fail('pods', time)).toBe(false));
        expect(window.fail('pods', 60_003)).toBe(false);
    });
    it('resets stopped watches and accepts quiet healthy connections', () => {
        const window = new WatchFailureWindow();
        [0, 1, 2, 3].forEach(time => window.fail('pods', time));
        window.status('pods', 'stopped', 4);
        expect(window.fail('pods', 5)).toBe(false);
        window.status('pods', 'connected', 6);
        expect(window.fail('pods', 60_006)).toBe(false);
    });
    it('recognizes short closures and transport errors', () => {
        expect(isImmediateWatchClosure({ premature: true, receivedAny: false, openDurationMillis: 250 })).toBe(true);
        expect(isImmediateWatchClosure({ premature: true, receivedAny: true })).toBe(false);
        expect(isImmediateWatchClosure({ premature: true, openDurationMillis: 30_000 })).toBe(false);
        expect(isStreamTransportError('unexpected EOF')).toBe(true);
        expect(isStreamTransportError('Forbidden')).toBe(false);
    });
    it('restores context preferences and validates intervals', () => {
        const values = new Map([['kubikles_connection_mode_a', 'polling'], ['kubikles_connection_mode_b', 'manual'],
            ['kubikles_polling_interval_a', '30'], ['kubikles_polling_interval_b', 'invalid']]);
        const storage = { getItem: (key: string) => values.get(key) ?? null };
        expect(restoreConnectionMode('a', storage)).toBe('polling');
        expect(restoreConnectionMode('b', storage)).toBe('manual');
        expect(restoreConnectionMode('new', storage)).toBe('streaming');
        expect(restorePollingInterval('a', storage)).toBe(30_000);
        expect(restorePollingInterval('b', storage)).toBe(10_000);
    });
});
