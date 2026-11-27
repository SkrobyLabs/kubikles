import { describe, expect, it } from 'vitest';
import { INITIAL_FIRST_ID, LogBuffer } from './logBuffer';

const ts = (n: number) => `2024-05-01T10:00:${String(n % 60).padStart(2, '0')}.${String(Math.floor(n / 60)).padStart(9, '0')}Z`;
const e = (n: number, content = `line ${n}`) => ({ timestamp: ts(n), content, source: 'stream' });

describe('LogBuffer appends', () => {
    it('appends in place and tracks the newest timestamp', () => {
        const buf = new LogBuffer(100);
        buf.replace([e(1), e(2)]);
        const arr = buf.entries;
        expect(buf.append([e(3), { timestamp: '', content: 'raw', source: 'stream' }])).toEqual({ added: 2, trimmed: 0 });
        expect(buf.entries).toBe(arr);
        expect(buf.lastTimestamp).toBe(ts(3));
        expect(buf.length).toBe(4);
    });

    it('appendLive drops replays against the pre-batch cursor only', () => {
        const buf = new LogBuffer(100);
        buf.replace([e(1), e(2), e(3)]);
        buf.appendLive([e(2), e(4), e(4, 'same'), e(3, 'older than batch peer')]);
        expect(buf.entries.map(x => x.content)).toEqual(['line 1', 'line 2', 'line 3', 'line 4', 'same']);
    });

    it('appendUnique skips duplicates by timestamp+content, scanning only the tail', () => {
        const buf = new LogBuffer(100);
        buf.replace([e(1), e(2), e(3)]);
        expect(buf.appendUnique([e(3), e(4)]).added).toBe(1);
        expect(buf.appendUnique([e(3, 'different text at same ts')]).added).toBe(1);
        expect(buf.entries.map(x => x.content)).toEqual(['line 1', 'line 2', 'line 3', 'line 4', 'different text at same ts']);
    });
});

describe('LogBuffer cap and ids', () => {
    it('trims the head in amortised chunks once past cap plus slack', () => {
        const buf = new LogBuffer(1000);
        buf.replace([]);
        const first = buf.entries;
        buf.append(Array.from({ length: 1000 }, (_, i) => e(i)));
        buf.append(Array.from({ length: 400 }, (_, i) => e(1000 + i)));
        // Within slack: nothing trimmed, same array.
        expect(buf.entries).toBe(first);
        expect(buf.firstId).toBe(INITIAL_FIRST_ID);
        const result = buf.append(Array.from({ length: 200 }, (_, i) => e(1400 + i)));
        expect(result.trimmed).toBe(600);
        expect(buf.length).toBe(1000);
        expect(buf.trimmedTotal).toBe(600);
        expect(buf.firstId).toBe(INITIAL_FIRST_ID + 600);
        expect(buf.entries).not.toBe(first);
        // Ids stay attached to their lines.
        expect(buf.entries[0].content).toBe('line 600');
        expect(buf.entries[buf.length - 1].content).toBe('line 1599');
    });

    it('prepend lowers firstId without changing existing ids and reduces the trimmed count', () => {
        const buf = new LogBuffer(1000);
        buf.replace([e(10), e(11)]);
        buf.trimmedTotal = 5;
        buf.prepend([e(8), e(9)]);
        expect(buf.firstId).toBe(INITIAL_FIRST_ID - 2);
        expect(buf.entries[2].content).toBe('line 10');
        expect(buf.firstId + 2).toBe(INITIAL_FIRST_ID);
        expect(buf.trimmedTotal).toBe(3);
    });

    it('clearTrimmed forgets the trimmed count without touching ids', () => {
        const buf = new LogBuffer(1000);
        buf.replace([e(10)]);
        buf.trimmedTotal = 7;
        const firstId = buf.firstId;
        buf.clearTrimmed();
        expect(buf.trimmedTotal).toBe(0);
        expect(buf.firstId).toBe(firstId);
    });

    it('raiseCap keeps explicitly loaded history through later live appends', () => {
        const buf = new LogBuffer(1000);
        buf.replace(Array.from({ length: 3000 }, (_, i) => e(i)));
        buf.raiseCap();
        buf.append(Array.from({ length: 500 }, (_, i) => e(3000 + i)));
        expect(buf.length).toBe(3500);
        expect(buf.trimmedTotal).toBe(0);
    });

    it('replace resets ids, cap and counters', () => {
        const buf = new LogBuffer(1000);
        buf.replace(Array.from({ length: 2000 }, (_, i) => e(i)));
        buf.raiseCap();
        buf.replace([e(1)]);
        buf.append(Array.from({ length: 1600 }, (_, i) => e(10 + i)));
        expect(buf.firstId).toBeGreaterThan(INITIAL_FIRST_ID);
        expect(buf.trimmedTotal).toBeGreaterThan(0);
    });

    it('snapshot reports count and ids', () => {
        const buf = new LogBuffer(1000);
        buf.replace([e(1), e(2)]);
        expect(buf.snapshot()).toMatchObject({ count: 2, firstId: INITIAL_FIRST_ID, trimmedTotal: 0 });
        expect(buf.firstTimestamp()).toBe(ts(1));
    });
});
