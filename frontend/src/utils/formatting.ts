/** Ages below this many seconds are shown to the second ("4m 12s"). */
export const AGE_SECONDS_UNTIL = 600;

/**
 * Milliseconds until the text formatAge returns for this timestamp next changes: the next whole
 * second while seconds are shown, then the next whole minute, then the next whole hour.
 */
export const nextAgeChangeMs = (timestamp: string | undefined, now: number = Date.now()): number => {
    const start = timestamp ? new Date(timestamp).getTime() : NaN;
    const elapsed = now - start;
    if (!Number.isFinite(elapsed) || elapsed < 0) return 1000;
    const unit = elapsed < AGE_SECONDS_UNTIL * 1000 ? 1000 : elapsed < 24 * 3600_000 ? 60_000 : 3600_000;
    return unit - (elapsed % unit);
};

/**
 * When something happened, in local time, with only the parts that add information: the time
 * alone for today, day and month added for another day this year, the year added beyond that.
 * Date order follows the system locale (or the one given); the clock is always 24-hour with seconds.
 */
export const formatWhen = (timestamp: string | undefined, now: Date = new Date(), locale?: string): string => {
    if (!timestamp) return '';
    const at = new Date(timestamp);
    if (Number.isNaN(at.getTime())) return '';
    const time = at.toLocaleTimeString(locale, { hour: '2-digit', minute: '2-digit', second: '2-digit', hourCycle: 'h23' });
    const sameYear = at.getFullYear() === now.getFullYear();
    if (sameYear && at.getMonth() === now.getMonth() && at.getDate() === now.getDate()) return time;
    const date = at.toLocaleDateString(locale, sameYear
        ? { day: '2-digit', month: '2-digit' }
        : { day: '2-digit', month: '2-digit', year: 'numeric' });
    return `${date}, ${time}`;
};

export const formatAge = (timestamp: string | undefined): string => {
    if (!timestamp) return '';
    const start = new Date(timestamp).getTime();
    const now = new Date().getTime();
    const diff = Math.floor((now - start) / 1000); // seconds

    if (diff < 60) return `${diff}s`;

    const minutes = Math.floor(diff / 60);
    // Young resources are the ones being watched, so they keep their seconds for a while.
    if (diff < AGE_SECONDS_UNTIL) return `${minutes}m ${diff % 60}s`;
    if (minutes < 60) return `${minutes}m`;

    const hours = Math.floor(minutes / 60);
    const remainingMinutes = minutes % 60;
    if (hours < 24) {
        return remainingMinutes > 0 ? `${hours}h ${remainingMinutes}m` : `${hours}h`;
    }

    const days = Math.floor(hours / 24);
    const remainingHours = hours % 24;

    if (days < 365) {
        return remainingHours > 0 ? `${days}d ${remainingHours}h` : `${days}d`;
    }

    const years = Math.floor(days / 365);
    const remainingDays = days % 365;
    return remainingDays > 0 ? `${years}y ${remainingDays}d` : `${years}y`;
};

export const formatBytes = (bytes: number | null | undefined): string => {
    if (bytes === 0 || bytes == null) return '0 B';
    const k = 1000;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(Math.abs(bytes)) / Math.log(k));
    const value = bytes / Math.pow(k, i);
    return `${value.toFixed(i > 0 ? 1 : 0)} ${sizes[Math.min(i, sizes.length - 1)]}`;
};

export const formatCpu = (millicores: number | null | undefined): string => {
    if (millicores == null || millicores === 0) return '0m';
    if (millicores >= 1000) {
        return `${(millicores / 1000).toFixed(1)} cores`;
    }
    return `${Math.round(millicores)}m`;
};

// formatChartTime formats a metrics-chart axis label, choosing date vs time
// granularity based on the selected duration window.
export const formatChartTime = (timestamp: string, duration: string): string => {
    const date = new Date(timestamp);
    if (duration === '30d' || duration === 'all') {
        return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
    }
    if (duration === '7d' || duration === '24h') {
        return date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
    }
    return date.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' });
};
