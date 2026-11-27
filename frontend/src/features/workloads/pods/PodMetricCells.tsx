import React from 'react';
import AggregateResourceBar from '~/components/shared/AggregateResourceBar';
import { formatBytes, formatCpu } from '~/utils/formatting';
import { useStoreValue, type ValueStore } from '~/utils/valueStore';
import { POD_RECORD_FIELDS, fromRecord, parseMetricsRecord, type PodMetricData } from '~/utils/metricsTransform';
import { podMetricKey } from './semantics';

export { podMetricKey } from './semantics';

export interface PodMetricsSnapshot {
    metrics: Record<string, any>;
    available: boolean | null;
}

export type PodMetricsStore = ValueStore<PodMetricsSnapshot>;

/** CPU / memory bars of one pod sample; N/A when the source is unavailable, "--" without a sample. */
export const PodMetricBars = React.memo(function PodMetricBars({ m, available, kind }: { m: any; available: boolean | null; kind: 'cpu' | 'memory' }) {
    if (available === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
    if (!m) return <span className="text-gray-500 text-xs">--</span>;
    if (kind === 'cpu') {
        return (
            <AggregateResourceBar
                usagePercent={m.cpuPercent}
                reservedPercent={m.cpuReservedPercent}
                committedPercent={m.cpuCommittedPercent}
                type="cpu"
                label="CPU"
                usageValue={m.cpuUsage}
                reservedValue={m.cpuRequested}
                committedValue={m.cpuCommitted}
                capacityValue={m.nodeCpuCapacity}
                formatValue={formatCpu}
            />
        );
    }
    return (
        <AggregateResourceBar
            usagePercent={m.memPercent}
            reservedPercent={m.memReservedPercent}
            committedPercent={m.memCommittedPercent}
            type="memory"
            label="Memory"
            usageValue={m.memoryUsage}
            reservedValue={m.memRequested}
            committedValue={m.memCommitted}
            capacityValue={m.nodeMemCapacity}
            formatValue={formatBytes}
        />
    );
});

/**
 * CPU / memory cell. It reads metrics from a store instead of from column closures, so a
 * metrics tick re-renders only these cells and leaves the column definitions (and therefore
 * the table's filter/sort pipeline) untouched.
 */
export const PodMetricCell = React.memo(function PodMetricCell({ store, item, kind }: { store: PodMetricsStore; item: any; kind: 'cpu' | 'memory' }) {
    const key = podMetricKey(item);
    const m = useStoreValue(store, s => s.metrics[key]);
    const available = useStoreValue(store, s => s.available);
    return <PodMetricBars m={m} available={available} kind={kind} />;
});

/** The same cell for a windowed row: the sample is the row's metrics cell, availability comes from the view state. */
export const PodMetricViewCell = React.memo(function PodMetricViewCell({ availability, item, kind }: { availability: ValueStore<boolean | null>; item: any; kind: 'cpu' | 'memory' }) {
    const available = useStoreValue(availability, a => a);
    const m = fromRecord<keyof PodMetricData & string>(POD_RECORD_FIELDS as readonly any[], parseMetricsRecord(item.cells?.metrics));
    return <PodMetricBars m={m} available={available} kind={kind} />;
});
