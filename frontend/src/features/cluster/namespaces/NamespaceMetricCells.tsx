import React from 'react';
import AggregateResourceBar from '~/components/shared/AggregateResourceBar';
import { formatBytes, formatCpu } from '~/utils/formatting';
import { useStoreValue, type ValueStore } from '~/utils/valueStore';
import { NAMESPACE_RECORD_FIELDS, fromRecord, parseMetricsRecord } from '~/utils/metricsTransform';

export interface NamespaceMetricsSnapshot {
    metrics: Record<string, any>;
    available: boolean | null;
}

/** CPU / memory bars of one namespace aggregate; N/A when the source is unavailable, "--" without a sample. */
export const NamespaceMetricBars = React.memo(function NamespaceMetricBars({ m, available, kind }: { m: any; available: boolean | null; kind: 'cpu' | 'memory' }) {
    if (available === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
    if (!m) return <span className="text-gray-500 text-xs">--</span>;
    if (kind === 'cpu') {
        return (
            <AggregateResourceBar
                usagePercent={m.cpuUsagePercent}
                reservedPercent={m.cpuReservedPercent}
                committedPercent={m.cpuCommittedPercent}
                type="cpu"
                label="CPU"
                usageValue={m.cpuUsage}
                reservedValue={m.cpuRequested}
                committedValue={m.cpuCommitted}
                formatValue={formatCpu}
            />
        );
    }
    return (
        <AggregateResourceBar
            usagePercent={m.memUsagePercent}
            reservedPercent={m.memReservedPercent}
            committedPercent={m.memCommittedPercent}
            type="memory"
            label="Memory"
            usageValue={m.memUsage}
            reservedValue={m.memRequested}
            committedValue={m.memCommitted}
            formatValue={formatBytes}
        />
    );
});

export const NamespaceMetricCell = React.memo(function NamespaceMetricCell({ store, item, kind }: { store: ValueStore<NamespaceMetricsSnapshot>; item: any; kind: 'cpu' | 'memory' }) {
    const name = item.metadata?.name;
    const m = useStoreValue(store, s => s.metrics[name]);
    const available = useStoreValue(store, s => s.available);
    return <NamespaceMetricBars m={m} available={available} kind={kind} />;
});

/** The same cell for a windowed row: the aggregate is the row's metrics cell, availability comes from the view state. */
export const NamespaceMetricViewCell = React.memo(function NamespaceMetricViewCell({ availability, item, kind }: { availability: ValueStore<boolean | null>; item: any; kind: 'cpu' | 'memory' }) {
    const available = useStoreValue(availability, a => a);
    const m = fromRecord(NAMESPACE_RECORD_FIELDS, parseMetricsRecord(item.cells?.metrics));
    return <NamespaceMetricBars m={m} available={available} kind={kind} />;
});
