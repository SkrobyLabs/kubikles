/**
 * Bounded, append-optimised store for log entries.
 *
 * The previous design kept logs in React state and re-copied the whole array
 * for every streamed batch, which is O(N) per batch and unbounded in memory.
 * Here appends mutate one array in place (O(batch)) and React is told about
 * them through immutable snapshots published at most once per frame.
 *
 * Line ids: every line has an id equal to `firstId + position`. Appending does
 * not change ids, prepending older history lowers `firstId`, and trimming the
 * head raises it, so an id keeps naming the same line for the buffer's whole
 * life. Virtuoso's `firstItemIndex` uses the same convention.
 */

export const LIVE_LOG_CAP = 50_000;
/** Initial id of the first line; leaves room to prepend history without going negative. */
export const INITIAL_FIRST_ID = 1_000_000;

export interface LogBufferSnapshot {
    /** The backing array. Positions below `count` never change while this identity is current. */
    entries: any[];
    count: number;
    firstId: number;
    /** Lines dropped from the head by the live cap and not re-loaded since. */
    trimmedTotal: number;
}

export class LogBuffer {
    entries: any[] = [];
    firstId = INITIAL_FIRST_ID;
    trimmedTotal = 0;
    lastTimestamp = '';
    private baseCap: number;
    private cap: number;

    constructor(cap: number = LIVE_LOG_CAP) {
        this.baseCap = cap;
        this.cap = cap;
    }

    get length() {
        return this.entries.length;
    }

    /** Trimming copies the array, so let it overshoot a little to amortise the copy. */
    private slack() {
        return Math.max(500, Math.floor(this.cap / 10));
    }

    private recomputeLastTimestamp() {
        this.lastTimestamp = '';
        for (let i = this.entries.length - 1; i >= 0; i--) {
            if (this.entries[i].timestamp) { this.lastTimestamp = this.entries[i].timestamp; break; }
        }
    }

    firstTimestamp(): string {
        for (let i = 0; i < this.entries.length; i++) {
            if (this.entries[i].timestamp) return this.entries[i].timestamp;
        }
        return '';
    }

    /** Replace everything (fresh fetch, jump, clear). Resets ids and the cap. */
    replace(entries: any[]) {
        this.entries = entries;
        this.firstId = INITIAL_FIRST_ID;
        this.trimmedTotal = 0;
        this.cap = this.baseCap;
        this.recomputeLastTimestamp();
    }

    /**
     * The user explicitly asked for more history ("load older", "load all"):
     * make room for it plus a full live window, so the next streamed batch
     * does not throw away what they just loaded.
     */
    raiseCap() {
        this.cap = Math.max(this.cap, this.entries.length + this.baseCap);
    }

    /** Older lines turned out to be unavailable (rotated away), so stop advertising them. */
    clearTrimmed() {
        this.trimmedTotal = 0;
    }

    /** Add older history at the head; existing ids are unchanged. */
    prepend(older: any[]) {
        if (older.length === 0) return;
        const hadTimestamp = this.lastTimestamp !== '';
        this.entries = older.concat(this.entries);
        this.firstId -= older.length;
        this.trimmedTotal = Math.max(0, this.trimmedTotal - older.length);
        if (!hadTimestamp) this.recomputeLastTimestamp();
    }

    /** Append at the tail, trimming the head if the cap is exceeded. Returns lines added and trimmed. */
    append(items: any[]): { added: number; trimmed: number } {
        if (items.length === 0) return { added: 0, trimmed: 0 };
        const entries = this.entries;
        let last = this.lastTimestamp;
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            entries.push(item);
            if (item.timestamp) last = item.timestamp;
        }
        this.lastTimestamp = last;

        let trimmed = 0;
        if (entries.length > this.cap + this.slack()) {
            trimmed = entries.length - this.cap;
            // A new array identity tells consumers that positions shifted.
            this.entries = entries.slice(trimmed);
            this.firstId += trimmed;
            this.trimmedTotal += trimmed;
        }
        return { added: items.length, trimmed };
    }

    /**
     * Stream append: drop timestamped lines at or before the newest line we
     * already hold (reconnect replays). The comparison is against the cursor
     * before the batch, not within it, matching the previous behaviour.
     */
    appendLive(incoming: any[]) {
        const cursor = this.lastTimestamp;
        const fresh = cursor ? incoming.filter((e: any) => !e.timestamp || e.timestamp > cursor) : incoming;
        return this.append(fresh);
    }

    /** Poll append: skip lines already present (same timestamp and content). */
    appendUnique(incoming: any[]) {
        if (incoming.length === 0) return { added: 0, trimmed: 0 };
        let minTs = '';
        for (const entry of incoming) {
            if (entry.timestamp) { minTs = entry.timestamp; break; }
        }
        // A duplicate must share a timestamp with an incoming line, so only the tail can hold one.
        const seen = new Set<string>();
        for (let i = this.entries.length - 1; i >= 0; i--) {
            const entry = this.entries[i];
            if (entry.timestamp && minTs && entry.timestamp < minTs) break;
            seen.add(`${entry.timestamp}\u0000${entry.content}`);
        }
        const fresh = incoming.filter((entry: any) => {
            const key = `${entry.timestamp}\u0000${entry.content}`;
            if (seen.has(key)) return false;
            seen.add(key);
            return true;
        });
        return this.append(fresh);
    }

    snapshot(): LogBufferSnapshot {
        return { entries: this.entries, count: this.entries.length, firstId: this.firstId, trimmedTotal: this.trimmedTotal };
    }
}
