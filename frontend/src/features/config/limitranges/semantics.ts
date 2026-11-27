import type { SemanticsMap } from '~/utils/columnSemantics';

export const limitRangeCount = (lr: any) => lr.spec?.limits?.length ?? 0;

/** Sort/filter semantics of the LimitRange list columns. */
export const limitRangeSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    limits: { getValue: limitRangeCount },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
