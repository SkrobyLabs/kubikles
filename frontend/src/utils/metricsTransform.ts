// Pure transforms of the metrics results into the maps the lists read. The legacy metrics hooks
// (usePodMetrics, useNodeMetrics, useNamespaceMetrics) and the golden-parity generator import
// them, and the Go overlay sources (pkg/overlays in the root package) reproduce them number for
// number: the rounding is JavaScript Math.round, so that is what the golden files record.

export interface PodMetricData {
    cpuPercent: number;
    memPercent: number;
    cpuReservedPercent: number;
    memReservedPercent: number;
    cpuCommittedPercent: number;
    memCommittedPercent: number;
    cpuUsage: number;
    memoryUsage: number;
    cpuRequested: number;
    memRequested: number;
    cpuCommitted: number;
    memCommitted: number;
    nodeCpuCapacity: number;
    nodeMemCapacity: number;
    nodeName: string;
}

export interface NodeMetricData {
    cpuPercent: number;
    memPercent: number;
    cpuReservedPercent: number;
    memReservedPercent: number;
    cpuCommittedPercent: number;
    memCommittedPercent: number;
    podPercent: number;
    cpuUsage: number;
    memoryUsage: number;
    cpuCapacity: number;
    memCapacity: number;
    cpuRequested: number;
    memRequested: number;
    cpuCommitted: number;
    memCommitted: number;
    podCount: number;
    podCapacity: number;
}

export interface NamespaceMetricData {
    cpuUsage: number;
    memUsage: number;
    cpuRequested: number;
    memRequested: number;
    cpuCommitted: number;
    memCommitted: number;
    podCount: number;
    cpuUsagePercent: number;
    memUsagePercent: number;
    cpuReservedPercent: number;
    memReservedPercent: number;
    cpuCommittedPercent: number;
    memCommittedPercent: number;
}

export type PodMetricsMap = Record<string, PodMetricData>;
export type NodeMetricsMap = Record<string, NodeMetricData>;
export type NamespaceMetricsMap = Record<string, NamespaceMetricData>;

const pct = (value: number, capacity: number): number => (capacity > 0 ? Math.round((value / capacity) * 100) : 0);

/** metrics-server / Prometheus pod result to the map keyed "namespace/name". */
export function transformPodMetrics(result: any): PodMetricsMap {
    const map: PodMetricsMap = {};
    for (const m of (result?.metrics || [])) {
        map[`${m.namespace}/${m.name}`] = {
            cpuPercent: pct(m.cpuUsage, m.nodeCpuCapacity),
            memPercent: pct(m.memoryUsage, m.nodeMemCapacity),
            cpuReservedPercent: pct(m.cpuRequested, m.nodeCpuCapacity),
            memReservedPercent: pct(m.memRequested, m.nodeMemCapacity),
            cpuCommittedPercent: pct(m.cpuCommitted, m.nodeCpuCapacity),
            memCommittedPercent: pct(m.memCommitted, m.nodeMemCapacity),
            cpuUsage: m.cpuUsage,
            memoryUsage: m.memoryUsage,
            cpuRequested: m.cpuRequested,
            memRequested: m.memRequested,
            cpuCommitted: m.cpuCommitted,
            memCommitted: m.memCommitted,
            nodeCpuCapacity: m.nodeCpuCapacity,
            nodeMemCapacity: m.nodeMemCapacity,
            nodeName: m.nodeName,
        };
    }
    return map;
}

/** Node result to the map keyed by node name. */
export function transformNodeMetrics(result: any): NodeMetricsMap {
    const map: NodeMetricsMap = {};
    for (const m of (result?.metrics || [])) {
        map[m.name] = {
            cpuPercent: pct(m.cpuUsage, m.cpuCapacity),
            memPercent: pct(m.memoryUsage, m.memCapacity),
            cpuReservedPercent: pct(m.cpuRequested, m.cpuCapacity),
            memReservedPercent: pct(m.memRequested, m.memCapacity),
            cpuCommittedPercent: pct(m.cpuCommitted, m.cpuCapacity),
            memCommittedPercent: pct(m.memCommitted, m.memCapacity),
            podPercent: pct(m.podCount, m.podCapacity),
            cpuUsage: m.cpuUsage,
            memoryUsage: m.memoryUsage,
            cpuCapacity: m.cpuCapacity,
            memCapacity: m.memCapacity,
            cpuRequested: m.cpuRequested,
            memRequested: m.memRequested,
            cpuCommitted: m.cpuCommitted,
            memCommitted: m.memCommitted,
            podCount: m.podCount,
            podCapacity: m.podCapacity,
        };
    }
    return map;
}

/** Cluster CPU and memory capacity: the sum over the node metrics. */
export function clusterCapacity(nodeMetricsMap: NodeMetricsMap): { cpuCapacity: number; memCapacity: number } {
    const nodes = Object.values(nodeMetricsMap);
    if (nodes.length === 0) return { cpuCapacity: 0, memCapacity: 0 };
    return nodes.reduce((acc, node) => ({
        cpuCapacity: acc.cpuCapacity + (node.cpuCapacity || 0),
        memCapacity: acc.memCapacity + (node.memCapacity || 0),
    }), { cpuCapacity: 0, memCapacity: 0 });
}

/** Pod metrics summed per namespace, as percentages of the cluster capacity. */
export function aggregateNamespaceMetrics(podMetricsMap: PodMetricsMap, nodeMetricsMap: NodeMetricsMap): NamespaceMetricsMap {
    const totals = clusterCapacity(nodeMetricsMap);
    const byNamespace: Record<string, any> = {};
    for (const [key, pod] of Object.entries(podMetricsMap)) {
        const namespace = key.split('/')[0];
        if (!byNamespace[namespace]) {
            byNamespace[namespace] = { cpuUsage: 0, memUsage: 0, cpuRequested: 0, memRequested: 0, cpuCommitted: 0, memCommitted: 0, podCount: 0 };
        }
        byNamespace[namespace].cpuUsage += pod.cpuUsage || 0;
        byNamespace[namespace].memUsage += pod.memoryUsage || 0;
        byNamespace[namespace].cpuRequested += pod.cpuRequested || 0;
        byNamespace[namespace].memRequested += pod.memRequested || 0;
        byNamespace[namespace].cpuCommitted += pod.cpuCommitted || 0;
        byNamespace[namespace].memCommitted += pod.memCommitted || 0;
        byNamespace[namespace].podCount += 1;
    }
    const result: NamespaceMetricsMap = {};
    for (const [namespace, m] of Object.entries(byNamespace)) {
        result[namespace] = {
            ...m,
            cpuUsagePercent: pct(m.cpuUsage, totals.cpuCapacity),
            memUsagePercent: pct(m.memUsage, totals.memCapacity),
            cpuReservedPercent: pct(m.cpuRequested, totals.cpuCapacity),
            memReservedPercent: pct(m.memRequested, totals.memCapacity),
            cpuCommittedPercent: pct(m.cpuCommitted, totals.cpuCapacity),
            memCommittedPercent: pct(m.memCommitted, totals.memCapacity),
        };
    }
    return result;
}

// The records the Go overlay sources publish and the view cells carry: fixed-order number
// vectors. The order is a wire contract with pkg/rows (overlay_metrics.go).
export const POD_RECORD_FIELDS = [
    'cpuPercent', 'memPercent', 'cpuReservedPercent', 'memReservedPercent', 'cpuCommittedPercent', 'memCommittedPercent',
    'cpuUsage', 'memoryUsage', 'cpuRequested', 'memRequested', 'cpuCommitted', 'memCommitted', 'nodeCpuCapacity', 'nodeMemCapacity',
] as const;

export const NODE_RECORD_FIELDS = [
    'cpuPercent', 'memPercent', 'cpuReservedPercent', 'memReservedPercent', 'cpuCommittedPercent', 'memCommittedPercent', 'podPercent',
    'cpuUsage', 'memoryUsage', 'cpuCapacity', 'memCapacity', 'cpuRequested', 'memRequested', 'cpuCommitted', 'memCommitted', 'podCount', 'podCapacity',
] as const;

export const NAMESPACE_RECORD_FIELDS = [
    'cpuUsagePercent', 'memUsagePercent', 'cpuReservedPercent', 'memReservedPercent', 'cpuCommittedPercent', 'memCommittedPercent',
    'cpuUsage', 'memUsage', 'cpuRequested', 'memRequested', 'cpuCommitted', 'memCommitted', 'podCount',
] as const;

export function recordOf<K extends string>(fields: readonly K[], m: Record<K, number>): number[] {
    return fields.map(f => m[f]);
}

/** Inverse of recordOf: a record read from a view cell back to the named fields the cells render. */
export function fromRecord<K extends string>(fields: readonly K[], record: readonly number[] | null | undefined): Record<K, number> | undefined {
    if (!record || record.length !== fields.length) return undefined;
    const out = {} as Record<K, number>;
    fields.forEach((f, i) => { out[f] = record[i]; });
    return out;
}

/**
 * A record as the text a view cell carries ("1,2,3"): a string sorts and compares like every
 * other display cell, and the cell stays one cheap value on the wire.
 */
export function metricsRecordText(record: readonly number[]): string {
    return record.join(',');
}

/** Inverse of metricsRecordText; undefined for an empty cell. */
export function parseMetricsRecord(text: string | null | undefined): number[] | undefined {
    if (!text) return undefined;
    return text.split(',').map(Number);
}
