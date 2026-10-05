import { describe, expect, it } from 'vitest';
import { createClockFormatter, filterClockTimeZones, getClockTimeZones } from './clockTimeZones';

describe('log toolbar clock', () => {
    it('includes UTC first and the actual system timezone without duplicates', () => {
        const zones = getClockTimeZones('Africa/Johannesburg');
        expect(zones.slice(0, 2)).toEqual(['UTC', 'Africa/Johannesburg']);
        expect(new Set(getClockTimeZones('Europe/London')).size).toBe(getClockTimeZones('Europe/London').length);
        expect(getClockTimeZones()).toContain(Intl.DateTimeFormat().resolvedOptions().timeZone);
        for (const zone of zones) expect(() => createClockFormatter(zone)).not.toThrow();
    });
    it('filters cities and regions regardless of case, whitespace or underscores', () => {
        const zones = getClockTimeZones();
        expect(filterClockTimeZones(zones, ' new york ')).toEqual(['America/New_York']);
        expect(filterClockTimeZones(zones, 'NEW_YORK')).toEqual(['America/New_York']);
        expect(filterClockTimeZones(zones, 'europe/')).toContain('Europe/London');
        expect(filterClockTimeZones(zones, 'nonexistent')).toEqual([]);
        expect(filterClockTimeZones(zones, '')).toEqual(zones);
    });
    it('formats UTC with an unambiguous date and 24-hour time', () => {
        expect(createClockFormatter('UTC').format(new Date('2026-10-05T00:02:03Z'))).toBe('05 Oct 2026, 00:02:03');
    });
    it('handles daylight saving and fractional timezone offsets', () => {
        const london = createClockFormatter('Europe/London');
        expect(london.format(new Date('2026-07-01T23:30:00Z'))).toBe('02 Jul 2026, 00:30:00');
        expect(london.format(new Date('2026-01-01T23:30:00Z'))).toBe('01 Jan 2026, 23:30:00');
        expect(createClockFormatter('Asia/Kolkata').format(new Date('2026-10-05T00:00:00Z'))).toBe('05 Oct 2026, 05:30:00');
    });
});
