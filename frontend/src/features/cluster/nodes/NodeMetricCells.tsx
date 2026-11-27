import React from 'react';
import AggregateResourceBar from '~/components/shared/AggregateResourceBar';
import ResourceBar from '~/components/shared/ResourceBar';
import { formatBytes, formatCpu } from '~/utils/formatting';
import { useStoreValue, type ValueStore } from '~/utils/valueStore';
import { NODE_RECORD_FIELDS, fromRecord, parseMetricsRecord } from '~/utils/metricsTransform';

export interface NodeMetricsSnapshot {
    metrics: Record<string, any>;
    available: boolean | null;
}

export type NodeMetricsStore = ValueStore<NodeMetricsSnapshot>;

type NodeMetricKind = 'cpu' | 'memory' | 'pods';

/** CPU / memory / pods bars of one node sample; N/A when the source is unavailable, "--" without a sample. */
export const NodeMetricBars = React.memo(function NodeMetricBars({ m, available, kind }: { m: any; available: boolean | null; kind: NodeMetricKind }) {
    if (available === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
    if (!m) return <span className="text-gray-500 text-xs">--</span>;
    if (kind === 'pods') {
        return <ResourceBar percent={m.podPercent} label="" tooltipLabel={`${m.podCount}/${m.podCapacity}`} color="bg-green-500" />;
    }
    if (kind === 'cpu') {
        return (
            <AggregateResourceBar
                usagePercent={m.cpuPercent}
                reservedPercent={m.cpuReservedPercent}
                committedPercent={m.cpuCommittedPercent}
                displayPercent={m.cpuPercent}
                type="cpu"
                label="CPU"
                usageValue={m.cpuUsage}
                reservedValue={m.cpuRequested}
                committedValue={m.cpuCommitted}
                capacityValue={m.cpuCapacity}
                formatValue={formatCpu}
            />
        );
    }
    return (
        <AggregateResourceBar
            usagePercent={m.memPercent}
            reservedPercent={m.memReservedPercent}
            committedPercent={m.memCommittedPercent}
            displayPercent={m.memPercent}
            type="memory"
            label="Memory"
            usageValue={m.memoryUsage}
            reservedValue={m.memRequested}
            committedValue={m.memCommitted}
            capacityValue={m.memCapacity}
            formatValue={formatBytes}
        />
    );
});

/** CPU / memory / pods cell reading from the metrics store; see PodMetricCell. */
export const NodeMetricCell = React.memo(function NodeMetricCell({ store, item, kind }: { store: NodeMetricsStore; item: any; kind: NodeMetricKind }) {
    const name = item.metadata?.name;
    const m = useStoreValue(store, s => s.metrics[name]);
    const available = useStoreValue(store, s => s.available);
    return <NodeMetricBars m={m} available={available} kind={kind} />;
});

/** The same cell for a windowed row: the sample is the row's metrics cell, availability comes from the view state. */
export const NodeMetricViewCell = React.memo(function NodeMetricViewCell({ availability, item, kind }: { availability: ValueStore<boolean | null>; item: any; kind: NodeMetricKind }) {
    const available = useStoreValue(availability, a => a);
    const m = fromRecord(NODE_RECORD_FIELDS, parseMetricsRecord(item.cells?.metrics));
    return <NodeMetricBars m={m} available={available} kind={kind} />;
});
