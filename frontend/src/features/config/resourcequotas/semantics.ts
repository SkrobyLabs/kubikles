import type { SemanticsMap } from '~/utils/columnSemantics';

export const resourceQuotaCount = (quota: any) => Object.keys(quota.spec?.hard || {}).length;

/** Sort/filter semantics of the ResourceQuota list columns. */
export const resourceQuotaSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    resources: { getValue: resourceQuotaCount },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
