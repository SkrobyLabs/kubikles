import type { SemanticsMap } from '~/utils/columnSemantics';

export const getRoleRefText = (binding: any): string => `${binding.roleRef?.kind}/${binding.roleRef?.name}`;
export const getSubjectCount = (binding: any): number => (binding.subjects || []).length;

/** Sort/filter semantics of the cluster role binding list columns. */
export const clusterRoleBindingSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    roleRef: { getValue: (item: any) => getRoleRefText(item) },
    subjects: { getValue: (item: any) => getSubjectCount(item) },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
