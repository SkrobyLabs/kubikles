import type { SemanticsMap } from '~/utils/columnSemantics';

/** Sort/filter semantics of the Event list columns that derive from the object itself. */
export const eventSemantics = {
    type: { getValue: (item: any) => item.type || '' },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    involvedObject: {
        getValue: (item: any) => (item.involvedObject ? `${item.involvedObject.kind}/${item.involvedObject.name}` : ''),
    },
    message: { getValue: (item: any) => item.message || '' },
    count: { getValue: (item: any) => item.count || 1 },
    age: { getValue: (item: any) => item.firstTimestamp || item.metadata?.creationTimestamp },
    last: { getValue: (item: any) => item.lastTimestamp || item.metadata?.creationTimestamp },
    reason: { getValue: (item: any) => item.reason || '' },
    source: { getValue: (item: any) => item.source?.component || item.reportingController || '' },
    objectKind: { getValue: (item: any) => item.involvedObject?.kind || '' },
} satisfies SemanticsMap;
