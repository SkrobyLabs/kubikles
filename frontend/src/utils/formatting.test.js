import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { formatAge, formatBytes, formatChartTime } from './formatting';

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

    it('formats minutes', () => {
        // 5 minutes ago
        expect(formatAge('2024-01-15T11:55:00.000Z')).toBe('5m');
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
        // Exactly 60 seconds should show 1m
        expect(formatAge('2024-01-15T11:59:00.000Z')).toBe('1m');
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
