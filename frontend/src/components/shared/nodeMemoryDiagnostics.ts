export interface MemoryPoint { timestamp: number; value: number }
export interface MemoryContributor { namespace: string; pod: string; workingSet: MemoryPoint[] }

const points = (series?: MemoryPoint[]) => (series || []).filter(point => Number.isFinite(point.timestamp) && Number.isFinite(point.value));

export const latestPoint = (series?: MemoryPoint[]) => {
    const values = points(series).sort((a, b) => a.timestamp - b.timestamp);
    return values[values.length - 1];
};
export const minimumPoint = (series?: MemoryPoint[]) => points(series).reduce<MemoryPoint | undefined>((minimum, point) => !minimum || point.value < minimum.value ? point : minimum, undefined);
export const peakPoint = (series?: MemoryPoint[]) => points(series).reduce<MemoryPoint | undefined>((peak, point) => !peak || point.value > peak.value ? point : peak, undefined);

export function alignedValue(series: MemoryPoint[] | undefined, timestamp: number, stepMs: number): number | undefined {
    const tolerance = Math.max(stepMs / 2, 1);
    const match = points(series).reduce<MemoryPoint | undefined>((closest, point) => !closest || Math.abs(point.timestamp - timestamp) < Math.abs(closest.timestamp - timestamp) ? point : closest, undefined);
    return match && Math.abs(match.timestamp - timestamp) <= tolerance ? match.value : undefined;
}

export function pressureEpisodes(series?: MemoryPoint[]): number {
    let wasPressure = false;
    return points(series).sort((a, b) => a.timestamp - b.timestamp).reduce((episodes, point) => {
        const pressure = point.value > 0;
        const next = pressure && !wasPressure ? episodes + 1 : episodes;
        wasPressure = pressure;
        return next;
    }, 0);
}

export function freshness(lastSample: MemoryPoint | undefined, rangeEndMs: number, stepMs: number): 'unavailable' | 'fresh' | 'stale' {
    if (!lastSample) return 'unavailable';
    return rangeEndMs - lastSample.timestamp > stepMs * 2 ? 'stale' : 'fresh';
}

export function segmentSeries(series: MemoryPoint[] | undefined, stepMs: number): MemoryPoint[][] {
    const result: MemoryPoint[][] = [];
    for (const point of points(series).sort((a, b) => a.timestamp - b.timestamp)) {
        const current = result[result.length - 1];
        if (!current || point.timestamp - current[current.length - 1].timestamp > stepMs * 2) result.push([point]);
        else current.push(point);
    }
    return result;
}

export function contributorSummaries(contributors?: MemoryContributor[]) {
    return (contributors || []).map(contributor => ({ ...contributor, peak: peakPoint(contributor.workingSet), last: latestPoint(contributor.workingSet) }))
        .sort((a, b) => (b.peak?.value || 0) - (a.peak?.value || 0) || a.namespace.localeCompare(b.namespace) || a.pod.localeCompare(b.pod));
}

export const nonNegative = (value: number | undefined) => value === undefined ? undefined : Math.max(0, value);
