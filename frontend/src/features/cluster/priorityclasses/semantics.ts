import type { SemanticsMap } from '~/utils/columnSemantics';

export const DEFAULT_PREEMPTION_POLICY = 'PreemptLowerPriority';

/** Sort/filter semantics of the priority class list columns. */
export const priorityClassSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    value: { getValue: (item: any) => item.value || 0 },
    globalDefault: { getValue: (item: any) => item.globalDefault ? 'Yes' : 'No' },
    preemption: { getValue: (item: any) => item.preemptionPolicy || DEFAULT_PREEMPTION_POLICY },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
