export interface MemoryPoint { timestamp: number; value: number }
export interface MemoryContributor { namespace: string; pod: string; workingSet: MemoryPoint[] }
export interface TimelineLine { label: string; data?: MemoryPoint[]; condition?: boolean }
export interface TimelineTooltip { timestamp: number; values: Array<{ label: string; value: number; condition?: boolean }>; markers: Array<{ reason: string; severity: string }> }

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

export function timelineTimeTicks(startMs: number, endMs: number, count = 5): Array<{ timestamp: number; ratio: number }> {
    const safeCount = Math.max(2, count);
    const range = Math.max(endMs - startMs, 1);
    return Array.from({ length: safeCount }, (_, index) => ({
        timestamp: startMs + (range * index) / (safeCount - 1),
        ratio: index / (safeCount - 1),
    }));
}

export function integerTimeRange(startMs: number, endMs: number): { startMs: number; endMs: number } {
    return {
        startMs: Math.floor(Math.min(startMs, endMs)),
        endMs: Math.ceil(Math.max(startMs, endMs)),
    };
}

export function timelineValueTicks(maxValue: number, count = 5): Array<{ value: number; ratio: number }> {
    const safeCount = Math.max(2, count);
    const max = Math.max(maxValue, 1);
    return Array.from({ length: safeCount }, (_, index) => ({
        value: max - (max * index) / (safeCount - 1),
        ratio: index / (safeCount - 1),
    }));
}

export function visibleTimelineLines(lines: TimelineLine[], hiddenLabels: ReadonlySet<string>): TimelineLine[] {
    return lines.filter(line => !hiddenLabels.has(line.label));
}

export function timelineDomainMax(lines: TimelineLine[]): number {
    return Math.max(...lines.flatMap(line => points(line.data)).filter(point => point.value >= 0).map(point => point.value), 1) * 1.05;
}

export function timelineTooltip(lines: TimelineLine[], timestamp: number, stepMs: number, markers: Array<{ timestamp: number; reason: string; severity: string }> = []): TimelineTooltip {
    const tolerance = Math.max(stepMs / 2, 1);
    return {
        timestamp,
        values: lines.flatMap(line => {
            const value = alignedValue(line.data, timestamp, stepMs);
            return value === undefined ? [] : [{ label: line.label, value, condition: line.condition }];
        }).slice(0, 8),
        markers: markers.filter(marker => Math.abs(marker.timestamp - timestamp) <= tolerance).map(marker => ({ reason: marker.reason, severity: marker.severity })).slice(0, 3),
    };
}

export function contributorSummaries(contributors?: MemoryContributor[]) {
    return (contributors || []).map(contributor => ({ ...contributor, peak: peakPoint(contributor.workingSet), last: latestPoint(contributor.workingSet) }))
        .sort((a, b) => (b.peak?.value || 0) - (a.peak?.value || 0) || a.namespace.localeCompare(b.namespace) || a.pod.localeCompare(b.pod));
}

export const nonNegative = (value: number | undefined) => value === undefined ? undefined : Math.max(0, value);
