import React, { useMemo, useState } from 'react';
import { formatBytes } from '~/utils/formatting';
import type { EventMarker } from './metrics/MetricsChart';
import { contributorSummaries, freshness, latestPoint, minimumPoint, peakPoint, pressureEpisodes, segmentSeries, type MemoryPoint } from './nodeMemoryDiagnostics';

type Props = { memory: any; contributors?: any[]; rangeEndMs: number; stepMs: number; markers?: EventMarker[]; onZoomSelect?: (startMs: number, endMs: number) => void };
type Line = { label: string; color: string; data?: MemoryPoint[] };

function Timeline({ lines, stepMs, markers = [], onZoomSelect }: { lines: Line[]; stepMs: number; markers?: EventMarker[]; onZoomSelect?: (startMs: number, endMs: number) => void }) {
    const available = lines.filter(line => line.data?.length);
    const all = available.flatMap(line => line.data || []);
    if (!all.length) return <div className="h-64 flex items-center justify-center border border-border bg-background text-sm text-gray-500">This diagnostic source is unavailable for the selected range.</div>;
    const start = Math.min(...all.map(point => point.timestamp));
    const end = Math.max(...all.map(point => point.timestamp));
    const max = Math.max(...all.map(point => point.value), 1) * 1.05;
    const point = (item: MemoryPoint) => `${48 + ((item.timestamp - start) / Math.max(end - start, 1)) * 732},${16 + (1 - item.value / max) * 180}`;
    return <div className="border border-border bg-background" onDoubleClick={() => onZoomSelect?.(start, end)}>
        <svg viewBox="0 0 800 220" className="w-full h-64" preserveAspectRatio="none">
            {[0, 1, 2, 3, 4].map(index => <line key={index} x1="48" x2="780" y1={16 + index * 45} y2={16 + index * 45} className="stroke-gray-800" strokeWidth="1" />)}
            {markers.filter(marker => marker.timestamp >= start && marker.timestamp <= end).map((marker, index) => <line key={index} x1={48 + ((marker.timestamp - start) / Math.max(end - start, 1)) * 732} x2={48 + ((marker.timestamp - start) / Math.max(end - start, 1)) * 732} y1="16" y2="196" className="stroke-amber-500" strokeDasharray="3,3" />)}
            {available.flatMap(line => segmentSeries(line.data, stepMs).map((segment, index) => <path key={`${line.label}-${index}`} d={segment.map((item, itemIndex) => `${itemIndex ? 'L' : 'M'} ${point(item)}`).join(' ')} fill="none" className={line.color} strokeWidth="2" />))}
        </svg>
        <div className="flex flex-wrap gap-x-4 gap-y-1 px-3 pb-2 text-xs">{available.map(line => <span key={line.label} className="text-gray-300"><span className={`inline-block w-3 h-0.5 mr-1 ${line.color.replace('stroke-', 'bg-')}`} />{line.label}</span>)}</div>
    </div>;
}

const Summary = ({ label, point }: { label: string; point?: MemoryPoint }) => <div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">{label}</div><div className="text-sm text-gray-200">{point ? formatBytes(point.value) : 'Unavailable'}</div></div>;

export default function NodeMemoryDiagnostics({ memory, contributors, rangeEndMs, stepMs, markers, onZoomSelect }: Props) {
    const [view, setView] = useState<'pressure' | 'allocation' | 'breakdown'>('pressure');
    const pressure = memory?.memoryPressure || [];
    const headroom = memory?.kubeletAvailable || [];
    const lastPressure = latestPoint(pressure);
    const pressureFreshness = freshness(lastPressure, rangeEndMs, stepMs);
    const contributorRows = useMemo(() => contributorSummaries(contributors), [contributors]);
    const lines: Line[] = view === 'pressure'
        ? [{ label: 'Eviction headroom', color: 'stroke-lime-400', data: headroom }, { label: 'OS available', color: 'stroke-emerald-500', data: memory?.available }, { label: 'Root working set', color: 'stroke-purple-500', data: memory?.workingSet }, { label: 'Capacity', color: 'stroke-gray-400', data: memory?.capacity }]
        : view === 'allocation'
            ? [{ label: 'Root working set', color: 'stroke-purple-500', data: memory?.workingSet }, { label: 'Pod working set', color: 'stroke-blue-500', data: memory?.podWorkingSet }, { label: 'Unexplained', color: 'stroke-amber-500', data: memory?.unexplained }, { label: 'Requests', color: 'stroke-yellow-500', data: memory?.reserved }, { label: 'Allocatable', color: 'stroke-gray-400', data: memory?.allocatable }]
            : [{ label: 'Page cache', color: 'stroke-cyan-500', data: memory?.pageCache }, { label: 'Slab', color: 'stroke-rose-500', data: memory?.slab }, { label: 'Shmem/tmpfs', color: 'stroke-pink-500', data: memory?.sharedMemory }];
    const primary = view === 'pressure' ? headroom : view === 'allocation' ? memory?.unexplained : memory?.pageCache;

    return <section className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-border pb-3"><div className="flex gap-1 bg-surface-light p-0.5">{(['pressure', 'allocation', 'breakdown'] as const).map(item => <button key={item} onClick={() => setView(item)} className={`px-3 py-1 text-xs capitalize ${view === item ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}`}>{item}</button>)}</div><span className="text-xs text-gray-500">Double-click a timeline to zoom to its covered range.</span></div>
        {view === 'pressure' && <><div className="grid grid-cols-2 lg:grid-cols-4 gap-2"><Summary label="Current eviction headroom" point={latestPoint(headroom)} /><Summary label="Minimum eviction headroom" point={minimumPoint(headroom)} /><div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">MemoryPressure episodes</div><div className="text-sm text-gray-200">{pressure.length ? pressureEpisodes(pressure) : 'Unknown'}</div></div><div className="border border-border bg-surface px-3 py-2"><div className="text-[11px] text-gray-500">Pressure sample freshness</div><div className="text-sm text-gray-200 capitalize">{pressureFreshness}</div></div></div><Timeline lines={lines} stepMs={stepMs} markers={markers} onZoomSelect={onZoomSelect} /></>}
        {view === 'allocation' && <><div className="grid grid-cols-2 lg:grid-cols-4 gap-2"><Summary label="Root working set" point={latestPoint(memory?.workingSet)} /><Summary label="Pod working set" point={latestPoint(memory?.podWorkingSet)} /><Summary label="Unexplained working set" point={latestPoint(memory?.unexplained)} /><Summary label="Peak unexplained" point={peakPoint(memory?.unexplained)} /></div><Timeline lines={lines} stepMs={stepMs} markers={markers} onZoomSelect={onZoomSelect} /><div className="border border-border"><div className="px-3 py-2 text-sm text-gray-300">Historical pod contributors</div>{contributorRows.length ? contributorRows.map(row => <div key={`${row.namespace}/${row.pod}`} className="grid grid-cols-3 gap-2 border-t border-border px-3 py-2 text-xs"><span>{row.namespace}/{row.pod}</span><span>Peak {row.peak ? formatBytes(row.peak.value) : 'Unavailable'}</span><span>Last {row.last ? `${formatBytes(row.last.value)} at ${new Date(row.last.timestamp).toLocaleString()}` : 'Unavailable'}</span></div>) : <div className="border-t border-border px-3 py-2 text-xs text-gray-500">Contributor histories require retained cAdvisor pod working-set metrics.</div>}</div></>}
        {view === 'breakdown' && <><div className="grid grid-cols-2 lg:grid-cols-3 gap-2">{lines.map(line => <Summary key={line.label} label={`Current ${line.label}`} point={latestPoint(line.data)} />)}</div><Timeline lines={lines} stepMs={stepMs} markers={markers} onZoomSelect={onZoomSelect} /><p className="text-xs text-gray-500">Partial attribution only: page cache, slab, and shmem/tmpfs do not sum to total node memory usage.</p></>}
        {primary?.length > 1 && segmentSeries(primary, stepMs).length > 1 && <div className="text-xs text-amber-400">Stale or missing coverage is shown as gaps in the timeline.</div>}
    </section>;
}
