import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { formatAge, formatBytes, formatChartTime, formatWhen, nextAgeChangeMs } from './formatting';

describe('formatAge', () => {
    beforeEach(() => {
        // Mock Date.now to return a fixed timestamp: 2024-01-15T12:00:00.000Z
        vi.useFakeTimers();
        vi.setSystemTime(new Date('2024-01-15T12:00:00.000Z'));
    });

    afterEach(() => {
        vi.useRealTimers();
    });

    it('returns empty string for null/undefined', () => {
        expect(formatAge(null)).toBe('');
        expect(formatAge(undefined)).toBe('');
        expect(formatAge('')).toBe('');
    });

    it('formats seconds', () => {
        // 30 seconds ago
        expect(formatAge('2024-01-15T11:59:30.000Z')).toBe('30s');
        expect(formatAge('2024-01-15T11:59:55.000Z')).toBe('5s');
    });

    it('keeps seconds for the first ten minutes', () => {
        expect(formatAge('2024-01-15T11:59:00.000Z')).toBe('1m 0s');
        expect(formatAge('2024-01-15T11:55:00.000Z')).toBe('5m 0s');
        expect(formatAge('2024-01-15T11:50:01.000Z')).toBe('9m 59s');
        expect(formatAge('2024-01-15T11:50:00.000Z')).toBe('10m');
    });

    it('formats minutes', () => {
        // 45 minutes ago
        expect(formatAge('2024-01-15T11:15:00.000Z')).toBe('45m');
    });

    it('formats hours with remaining minutes', () => {
        // 2 hours 30 minutes ago
        expect(formatAge('2024-01-15T09:30:00.000Z')).toBe('2h 30m');
        // 5 hours ago (no remaining minutes)
        expect(formatAge('2024-01-15T07:00:00.000Z')).toBe('5h');
    });

    it('formats days with remaining hours', () => {
        // 3 days 12 hours ago
        expect(formatAge('2024-01-12T00:00:00.000Z')).toBe('3d 12h');
        // 7 days ago (no remaining hours)
        expect(formatAge('2024-01-08T12:00:00.000Z')).toBe('7d');
    });

    it('formats years with remaining days', () => {
        // 1 year 100 days ago
        expect(formatAge('2022-10-07T12:00:00.000Z')).toBe('1y 100d');
        // 2 years ago (no remaining days)
        expect(formatAge('2022-01-15T12:00:00.000Z')).toBe('2y');
    });

    it('handles edge case at 60 seconds', () => {
        // Exactly 60 seconds rolls over to minutes and keeps its seconds
        expect(formatAge('2024-01-15T11:59:00.000Z')).toBe('1m 0s');
    });

    it('handles edge case at 60 minutes', () => {
        // Exactly 60 minutes should show 1h
        expect(formatAge('2024-01-15T11:00:00.000Z')).toBe('1h');
    });

    it('handles edge case at 24 hours', () => {
        // Exactly 24 hours should show 1d
        expect(formatAge('2024-01-14T12:00:00.000Z')).toBe('1d');
    });
});

describe('formatBytes', () => {
    it('uses decimal labels for Kubernetes resource byte values', () => {
        expect(formatBytes(200_000_000)).toBe('200.0 MB');
        expect(formatBytes(1_000_000_000)).toBe('1.0 GB');
    });
});

describe('formatChartTime', () => {
    // Use a midday UTC timestamp so the date is stable across the timezones CI may run in.
    const ts = '2024-06-15T12:00:00.000Z';

    it('renders date-only labels (no clock time) for wide windows', () => {
        expect(formatChartTime(ts, 'all')).not.toContain(':');
        expect(formatChartTime(ts, '30d')).not.toContain(':');
    });

    it('includes clock time for mid-range windows', () => {
        expect(formatChartTime(ts, '7d')).toContain(':');
        expect(formatChartTime(ts, '24h')).toContain(':');
    });

    it('renders time-only labels for short/default windows', () => {
        const short = formatChartTime(ts, '1h');
        expect(short).toContain(':');
        // time-only: no month abbreviation like "Jun"
        expect(short).not.toMatch(/[A-Za-z]{3}/);
    });
});

describe('nextAgeChangeMs', () => {
    const now = new Date('2024-01-15T12:00:00.000Z').getTime();
    it('waits for the next whole second while seconds are shown', () => {
        expect(nextAgeChangeMs('2024-01-15T11:59:55.400Z', now)).toBe(400);
        expect(nextAgeChangeMs('2024-01-15T11:51:00.250Z', now)).toBe(250);
    });
    it('waits for the next whole minute, then the next whole hour', () => {
        expect(nextAgeChangeMs('2024-01-15T11:29:40.000Z', now)).toBe(40_000);
        expect(nextAgeChangeMs('2024-01-13T11:30:00.000Z', now)).toBe(30 * 60_000);
    });
    it('retries shortly for a missing or future timestamp', () => {
        expect(nextAgeChangeMs(undefined, now)).toBe(1000);
        expect(nextAgeChangeMs('2024-01-15T12:00:05.000Z', now)).toBe(1000);
    });
});

describe('formatWhen', () => {
    // Local-time constructors keep these independent of the machine's time zone, and the locale
    // is passed in so they do not depend on the machine's language either.
    const now = new Date(2026, 10, 28, 13, 31, 0);
    const iso = (...parts) => new Date(...parts).toISOString();

    it('shows only the time for today', () => {
        expect(formatWhen(iso(2026, 10, 28, 13, 11, 18), now, 'en-GB')).toBe('13:11:18');
        expect(formatWhen(iso(2026, 10, 28, 0, 0, 5), now, 'en-GB')).toBe('00:00:05');
    });
    it('adds day and month for another day this year', () => {
        expect(formatWhen(iso(2026, 10, 27, 23, 57, 59), now, 'en-GB')).toBe('27/11, 23:57:59');
    });
    it('adds the year for another year', () => {
        expect(formatWhen(iso(2025, 2, 4, 8, 5, 9), now, 'en-GB')).toBe('04/03/2025, 08:05:09');
    });
    it('follows the date order of the locale', () => {
        expect(formatWhen(iso(2026, 10, 27, 23, 57, 59), now, 'en-US')).toBe('11/27, 23:57:59');
    });
    it('returns an empty string for a missing or invalid timestamp', () => {
        expect(formatWhen(undefined, now)).toBe('');
        expect(formatWhen('not a date', now)).toBe('');
    });
});
