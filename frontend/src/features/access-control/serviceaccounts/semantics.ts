import type { SemanticsMap } from '~/utils/columnSemantics';

export const serviceAccountSecretCount = (item: any) => (item.secrets || []).length;

/** Sort/filter semantics of the ServiceAccount list columns. */
export const serviceAccountSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    secrets: { getValue: serviceAccountSecretCount },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
