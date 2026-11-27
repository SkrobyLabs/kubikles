import type { SemanticsMap } from '~/utils/columnSemantics';

export const getRoleRuleCount = (role: any): number => (role.rules || []).length;

/** Sort/filter semantics of the Role list columns. */
export const roleSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    rules: { getValue: (item: any) => getRoleRuleCount(item) },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
