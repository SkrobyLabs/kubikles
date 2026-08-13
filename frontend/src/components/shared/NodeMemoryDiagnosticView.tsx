import React, { useMemo, useState } from 'react';
import { formatBytes, formatChartTime } from '~/utils/formatting';
import type { EventMarker } from './metrics/MetricsChart';
import { alignedValue, contributorSummaries, freshness, latestPoint, minimumPoint, peakPoint, pressureEpisodes, segmentSeries, timelineTimeTicks, timelineTooltip, timelineValueTicks, type MemoryPoint, type TimelineLine } from './nodeMemoryDiagnostics';

type Props = { memory: any; contributors?: any[]; rangeStartMs: number; rangeEndMs: number; stepMs: number; markers?: EventMarker[]; onZoomSelect?: (startMs: number, endMs: number) => void };
type Line = TimelineLine & { color: string };

function Timeline({ lines, rangeStartMs, rangeEndMs, stepMs, markers = [], onZoomSelect }: { lines: Line[]; rangeStartMs: number; rangeEndMs: number; stepMs: number; markers?: EventMarker[]; onZoomSelect?: (startMs: number, endMs: number) => void }) {
    const plotLeft = 72;
    const plotRight = 780;
    const plotTop = 16;
    const plotBottom = 188;
    const [hoveredTimestamp, setHoveredTimestamp] = useState<number>();
    const [dragStart, setDragStart] = useState<number>();
    const [dragEnd, setDragEnd] = useState<number>();
    const available = lines.filter(line => line.data?.length);
    const all = available.flatMap(line => line.data || []);
    const fallbackStart = all.length ? Math.min(...all.map(point => point.timestamp)) : rangeStartMs;
    const fallbackEnd = all.length ? Math.max(...all.map(point => point.timestamp)) : rangeEndMs;
    const start = Number.isFinite(rangeStartMs) ? rangeStartMs : fallbackStart;
    const end = Number.isFinite(rangeEndMs) && rangeEndMs > start ? rangeEndMs : Math.max(fallbackEnd, start + Math.max(stepMs, 1));
    const max = Math.max(...all.filter(point => point.value >= 0).map(point => point.value), 1) * 1.05;
    const chartDuration = end - start >= 30 * 24 * 60 * 60 * 1000 ? '30d' : end - start >= 7 * 24 * 60 * 60 * 1000 ? '7d' : end - start >= 24 * 60 * 60 * 1000 ? '24h' : '1h';
    const timeLabel = (timestamp: number) => formatChartTime(new Date(timestamp).toISOString(), chartDuration);
    const coverageStart = all.length ? Math.max(start, Math.min(...all.map(point => point.timestamp))) : start;
    const coverageEnd = all.length ? Math.min(end, Math.max(...all.map(point => point.timestamp))) : end;
    const x = (timestamp: number) => plotLeft + ((timestamp - start) / Math.max(end - start, 1)) * (plotRight - plotLeft);
    const y = (value: number) => plotTop + (1 - value / max) * (plotBottom - plotTop);
    const point = (item: MemoryPoint) => `${x(item.timestamp)},${y(item.value)}`;
    const timestampAt = (event: React.MouseEvent<SVGSVGElement>) => {
        const bounds = event.currentTarget.getBoundingClientRect();
        const svgX = ((event.clientX - bounds.left) / Math.max(bounds.width, 1)) * 800;
        const ratio = Math.max(0, Math.min(1, (svgX - plotLeft) / (plotRight - plotLeft)));
        return start + ratio * (end - start);
    };
    const tooltip = hoveredTimestamp === undefined ? undefined : timelineTooltip(available, hoveredTimestamp, stepMs, markers);
    const selectedStart = dragStart === undefined || dragEnd === undefined ? undefined : Math.min(dragStart, dragEnd);
    const selectedEnd = dragStart === undefined || dragEnd === undefined ? undefined : Math.max(dragStart, dragEnd);
    const tooltipLeft = hoveredTimestamp === undefined ? 0 : Math.min(88, Math.max(2, (x(hoveredTimestamp) / 800) * 100));

    return <div className="relative border border-border bg-background">
        <svg viewBox="0 0 800 232" className="h-64 w-full cursor-crosshair select-none" preserveAspectRatio="none" onMouseMove={event => { const timestamp = timestampAt(event); setHoveredTimestamp(timestamp); if (dragStart !== undefined) setDragEnd(timestamp); }} onMouseLeave={() => { setHoveredTimestamp(undefined); setDragStart(undefined); setDragEnd(undefined); }} onMouseDown={event => { const timestamp = timestampAt(event); setDragStart(timestamp); setDragEnd(timestamp); }} onMouseUp={event => { const timestamp = timestampAt(event); if (dragStart !== undefined && Math.abs(timestamp - dragStart) > Math.max(stepMs / 4, 1)) onZoomSelect?.(Math.min(dragStart, timestamp), Math.max(dragStart, timestamp)); setDragStart(undefined); setDragEnd(undefined); }}>
            {timelineValueTicks(max).map(tick => <g key={tick.ratio}><line x1={plotLeft} x2={plotRight} y1={plotTop + tick.ratio * (plotBottom - plotTop)} y2={plotTop + tick.ratio * (plotBottom - plotTop)} className="stroke-gray-800" strokeWidth="1" /><text x={plotLeft - 8} y={plotTop + tick.ratio * (plotBottom - plotTop) + 3} textAnchor="end" className="fill-gray-500" fontSize="10">{formatBytes(tick.value)}</text></g>)}
            {timelineTimeTicks(start, end).map(tick => <g key={tick.ratio}><line x1={plotLeft + tick.ratio * (plotRight - plotLeft)} x2={plotLeft + tick.ratio * (plotRight - plotLeft)} y1={plotBottom} y2={plotBottom + 4} className="stroke-gray-700" strokeWidth="1" /><text x={plotLeft + tick.ratio * (plotRight - plotLeft)} y="212" textAnchor={tick.ratio === 0 ? 'start' : tick.ratio === 1 ? 'end' : 'middle'} className="fill-gray-500" fontSize="10">{timeLabel(tick.timestamp)}</text></g>)}
            {coverageStart > start && <rect x={plotLeft} y={plotTop} width={x(coverageStart) - plotLeft} height={plotBottom - plotTop} className="fill-gray-500 opacity-20" />}
            {coverageEnd < end && <rect x={x(coverageEnd)} y={plotTop} width={plotRight - x(coverageEnd)} height={plotBottom - plotTop} className="fill-gray-500 opacity-20" />}
            {!all.length && <rect x={plotLeft} y={plotTop} width={plotRight - plotLeft} height={plotBottom - plotTop} className="fill-gray-500 opacity-20" />}
            {available.filter(line => !line.condition).flatMap(line => segmentSeries(line.data, stepMs).map((segment, index) => <path key={`${line.label}-${index}`} d={segment.map((item, itemIndex) => `${itemIndex ? 'L' : 'M'} ${point(item)}`).join(' ')} fill="none" className={line.color} strokeWidth="2" />))}
            {available.filter(line => line.condition).flatMap(line => segmentSeries(line.data, stepMs).flatMap((segment, segmentIndex) => segment.flatMap((item, itemIndex) => {
                if (item.value <= 0) return [];
                const next = segment[itemIndex + 1];
                const bandEnd = Math.min(end, next ? next.timestamp : item.timestamp + stepMs);
                return bandEnd > item.timestamp ? <rect key={`${line.label}-${segmentIndex}-${item.timestamp}`} x={x(item.timestamp)} y={plotTop} width={x(bandEnd) - x(item.timestamp)} height="12" className={line.color.replace('stroke-', 'fill-')} /> : [];
            })))}
            {markers.filter(marker => marker.timestamp >= start && marker.timestamp <= end).map((marker, index) => <line key={index} x1={x(marker.timestamp)} x2={x(marker.timestamp)} y1={plotTop} y2={plotBottom} className="stroke-amber-500" strokeDasharray="3,3" />)}
            {hoveredTimestamp !== undefined && <line x1={x(hoveredTimestamp)} x2={x(hoveredTimestamp)} y1={plotTop} y2={plotBottom} className="stroke-gray-400" strokeDasharray="2,2" />}
            {selectedStart !== undefined && selectedEnd !== undefined && <rect x={x(selectedStart)} y={plotTop} width={x(selectedEnd) - x(selectedStart)} height={plotBottom - plotTop} className="fill-primary opacity-20" />}
        </svg>
        {tooltip && <div className="pointer-events-none absolute top-2 z-10 max-w-56 border border-border bg-surface px-2 py-1 text-xs shadow-lg" style={{ left: `${tooltipLeft}%`, transform: tooltipLeft > 68 ? 'translateX(-100%)' : undefined }}><div className="text-gray-400">{timeLabel(tooltip.timestamp)}</div>{tooltip.values.map(item => <div key={item.label} className="text-gray-200">{item.label}: {item.condition ? item.value > 0 ? 'True' : 'False' : formatBytes(item.value)}</div>)}{tooltip.markers.map(marker => <div key={`${marker.severity}-${marker.reason}`} className="text-amber-400">{marker.reason}</div>)}{!tooltip.values.length && !tooltip.markers.length && <div className="text-gray-500">No sample at this time</div>}</div>}
        <div className="flex flex-wrap gap-x-4 gap-y-1 px-3 pb-2 text-xs">{available.map(line => <span key={line.label} className="text-gray-300"><span className={`inline-block w-3 h-0.5 mr-1 ${line.color.replace('stroke-', 'bg-')}`} />{line.label}</span>)}{!all.length && <span className="text-gray-500">This diagnostic source is unavailable for the selected range.</span>}</div>
    </div>;
}

const Summary = ({ label, point }: { label: string; point?: MemoryPoint }) => <div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">{label}</div><div className="text-sm text-gray-200">{point ? formatBytes(point.value) : 'Unavailable'}</div></div>;

export default function NodeMemoryDiagnostics({ memory, contributors, rangeStartMs, rangeEndMs, stepMs, markers, onZoomSelect }: Props) {
    const [view, setView] = useState<'pressure' | 'allocation' | 'breakdown'>('pressure');
    const pressure = memory?.memoryPressure || [];
    const headroom = memory?.kubeletAvailable || [];
    const lastPressure = latestPoint(pressure);
    const pressureFreshness = freshness(lastPressure, rangeEndMs, stepMs);
    const contributorRows = useMemo(() => contributorSummaries(contributors), [contributors]);
    const currentHeadroom = latestPoint(headroom);
    const allocatableAtCurrentHeadroom = currentHeadroom ? alignedValue(memory?.allocatable, currentHeadroom.timestamp, stepMs) : undefined;
    const headroomPercent = currentHeadroom && allocatableAtCurrentHeadroom && allocatableAtCurrentHeadroom > 0 ? (currentHeadroom.value / allocatableAtCurrentHeadroom) * 100 : undefined;
    const currentPressure = pressureFreshness === 'fresh' && lastPressure ? lastPressure.value > 0 ? 'True' : 'False' : 'Unknown';
    const contributorColors = ['stroke-fuchsia-500', 'stroke-orange-400', 'stroke-teal-400', 'stroke-indigo-400', 'stroke-red-400'];
    const lines: Line[] = view === 'pressure'
        ? [{ label: 'Allocatable headroom', color: 'stroke-lime-400', data: headroom }, { label: 'OS available', color: 'stroke-emerald-500', data: memory?.available }, { label: 'Root working set', color: 'stroke-purple-500', data: memory?.workingSet }, { label: 'Capacity', color: 'stroke-gray-400', data: memory?.capacity }, { label: 'MemoryPressure=True', color: 'stroke-red-500', data: pressure, condition: true }]
        : view === 'allocation'
            ? [{ label: 'Root working set', color: 'stroke-purple-500', data: memory?.workingSet }, { label: 'Pod working set', color: 'stroke-blue-500', data: memory?.podWorkingSet }, { label: 'Unexplained', color: 'stroke-amber-500', data: memory?.unexplained }, { label: 'Requests', color: 'stroke-yellow-500', data: memory?.reserved }, { label: 'Allocatable', color: 'stroke-gray-400', data: memory?.allocatable }, ...contributorRows.slice(0, 5).map((contributor, index) => ({ label: `${contributor.namespace}/${contributor.pod}`, color: contributorColors[index], data: contributor.workingSet }))]
            : [{ label: 'Page cache', color: 'stroke-cyan-500', data: memory?.pageCache }, { label: 'Slab', color: 'stroke-rose-500', data: memory?.slab }, { label: 'Shmem/tmpfs', color: 'stroke-pink-500', data: memory?.sharedMemory }];
    const primary = view === 'pressure' ? headroom : view === 'allocation' ? memory?.unexplained : memory?.pageCache;

    return <section className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-3"><div className="flex gap-1 bg-surface-light p-0.5">{(['pressure', 'allocation', 'breakdown'] as const).map(item => <button key={item} onClick={() => setView(item)} className={`px-3 py-1 text-xs capitalize ${view === item ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}`}>{item}</button>)}</div></div>
        {view === 'pressure' && <><div className="grid grid-cols-2 lg:grid-cols-5 gap-2"><Summary label="Current allocatable headroom" point={currentHeadroom} /><Summary label="Minimum allocatable headroom" point={minimumPoint(headroom)} /><div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">Headroom of allocatable</div><div className="text-sm text-gray-200">{headroomPercent === undefined ? 'Unavailable' : `${headroomPercent.toFixed(1)}%`}</div></div><div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">Current MemoryPressure</div><div className="text-sm text-gray-200">{currentPressure}</div></div><div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">MemoryPressure episodes</div><div className="text-sm text-gray-200">{pressure.length ? pressureEpisodes(pressure) : 'Unknown'}</div></div><div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">Pressure sample freshness</div><div className="text-sm text-gray-200 capitalize">{pressureFreshness}</div></div></div><Timeline lines={lines} rangeStartMs={rangeStartMs} rangeEndMs={rangeEndMs} stepMs={stepMs} markers={markers} onZoomSelect={onZoomSelect} /></>}
        {view === 'allocation' && <><div className="grid grid-cols-2 lg:grid-cols-4 gap-2"><Summary label="Root working set" point={latestPoint(memory?.workingSet)} /><Summary label="Pod working set" point={latestPoint(memory?.podWorkingSet)} /><Summary label="Unexplained working set" point={latestPoint(memory?.unexplained)} /><Summary label="Peak unexplained" point={peakPoint(memory?.unexplained)} /></div><Timeline lines={lines} rangeStartMs={rangeStartMs} rangeEndMs={rangeEndMs} stepMs={stepMs} markers={markers} onZoomSelect={onZoomSelect} /><div className="border border-border"><div className="px-3 py-2 text-sm text-gray-300">Historical pod contributors</div>{contributorRows.length ? contributorRows.map(row => <div key={`${row.namespace}/${row.pod}`} className="grid grid-cols-3 gap-2 border-t border-border px-3 py-2 text-xs"><span>{row.namespace}/{row.pod}</span><span>Peak {row.peak ? formatBytes(row.peak.value) : 'Unavailable'}</span><span>Last {row.last ? `${formatBytes(row.last.value)} at ${new Date(row.last.timestamp).toLocaleString()}` : 'Unavailable'}</span></div>) : <div className="border-t border-border px-3 py-2 text-xs text-gray-500">Contributor histories require retained cAdvisor pod working-set metrics.</div>}</div></>}
        {view === 'breakdown' && <><div className="grid grid-cols-2 lg:grid-cols-3 gap-2">{lines.flatMap(line => [<Summary key={`current-${line.label}`} label={`Current ${line.label}`} point={latestPoint(line.data)} />, <Summary key={`peak-${line.label}`} label={`Peak ${line.label}`} point={peakPoint(line.data)} />])}</div><Timeline lines={lines} rangeStartMs={rangeStartMs} rangeEndMs={rangeEndMs} stepMs={stepMs} markers={markers} onZoomSelect={onZoomSelect} /><p className="text-xs text-gray-500">Partial attribution only: page cache, slab, and shmem/tmpfs do not sum to total node memory usage.</p></>}
        {primary?.length > 1 && segmentSeries(primary, stepMs).length > 1 && <div className="text-xs text-amber-400">Stale or missing coverage is shown as gaps in the timeline.</div>}
    </section>;
}
