export const COMMON_TIME_ZONES = [
    'UTC', 'Europe/London', 'Europe/Dublin', 'Europe/Paris', 'Europe/Berlin',
    'Europe/Helsinki', 'America/New_York', 'America/Chicago', 'America/Denver',
    'America/Los_Angeles', 'America/Phoenix', 'America/Toronto', 'America/Sao_Paulo',
    'Asia/Dubai', 'Asia/Kolkata', 'Asia/Singapore', 'Asia/Hong_Kong', 'Asia/Shanghai',
    'Asia/Tokyo', 'Asia/Seoul', 'Australia/Sydney', 'Australia/Perth', 'Pacific/Auckland',
];

export function getClockTimeZones(systemZone = Intl.DateTimeFormat().resolvedOptions().timeZone): string[] {
    return [...new Set(['UTC', systemZone, ...COMMON_TIME_ZONES].filter(Boolean))];
}

export function filterClockTimeZones(zones: string[], query: string): string[] {
    const normalize = (value: string) => value.replace(/_/g, ' ').toLowerCase();
    return zones.filter(zone => normalize(zone).includes(normalize(query.trim())));
}

export function createClockFormatter(timeZone: string): Intl.DateTimeFormat {
    return new Intl.DateTimeFormat('en-GB', {
        timeZone, year: 'numeric', month: 'short', day: '2-digit',
        hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23',
    });
}
