import type { SemanticsMap } from '~/utils/columnSemantics';

export const getRuleCount = (role: any): number => (role.rules || []).length;
export const isAggregated = (role: any): boolean => (role.aggregationRule?.clusterRoleSelectors || []).length > 0;

/** Sort/filter semantics of the cluster role list columns. */
export const clusterRoleSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    rules: { getValue: (item: any) => getRuleCount(item) },
    aggregation: { getValue: (item: any) => isAggregated(item) ? 'Yes' : 'No' },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
