import type { SemanticsMap } from '~/utils/columnSemantics';
import { NAMESPACE_RECORD_FIELDS, recordOf, metricsRecordText, type NamespaceMetricData } from '~/utils/metricsTransform';

// Get namespace status from conditions
export function getNamespaceStatus(namespace: any) {
    const phase = namespace.status?.phase;
    if (phase === 'Active') return 'Active';
    if (phase === 'Terminating') return 'Terminating';
    return phase || 'Unknown';
}

/** Where the metrics columns read their aggregate from (a store in the list, a fixture in the golden test). */
export type NamespaceMetricLookup = (item: any) => NamespaceMetricData | undefined;

/**
 * Sort/filter semantics of the namespace list columns. cpu and memory sort by the committed
 * percentage of the cluster capacity (-1 without a sample) and offer no usable filter: their
 * value is a number, so the select filter has nothing to list.
 */
export function createNamespaceSemantics(lookup: NamespaceMetricLookup) {
    return {
        name: { getValue: (item: any) => item.metadata?.name },
        cpu: { getValue: (item: any) => lookup(item)?.cpuCommittedPercent ?? -1 },
        memory: { getValue: (item: any) => lookup(item)?.memCommittedPercent ?? -1 },
        status: { getValue: (item: any) => getNamespaceStatus(item) },
        age: { getValue: (item: any) => item.metadata?.creationTimestamp },
        /** Not a column of the list: the view's display-only cell carrying the aggregate the metric bars draw. */
        metrics: {
            getValue: (item: any) => {
                const m = lookup(item);
                return m ? metricsRecordText(recordOf(NAMESPACE_RECORD_FIELDS, m)) : '';
            },
            filterable: false,
        },
    } satisfies SemanticsMap;
}

export const namespaceSemantics = createNamespaceSemantics(() => undefined);
