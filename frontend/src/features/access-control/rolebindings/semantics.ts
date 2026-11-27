import type { SemanticsMap } from '~/utils/columnSemantics';

export const getRoleBindingRefText = (binding: any): string => `${binding.roleRef?.kind}/${binding.roleRef?.name}`;
export const getRoleBindingSubjectCount = (binding: any): number => (binding.subjects || []).length;

/** Sort/filter semantics of the RoleBinding list columns. */
export const roleBindingSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    roleRef: { getValue: (item: any) => getRoleBindingRefText(item) },
    subjects: { getValue: (item: any) => getRoleBindingSubjectCount(item) },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
