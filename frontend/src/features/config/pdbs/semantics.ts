import type { SemanticsMap } from '~/utils/columnSemantics';

/** A number, a percentage string or the placeholder dash. */
export const pdbMinAvailable = (pdb: any) => pdb.spec?.minAvailable ?? '-';
export const pdbMaxUnavailable = (pdb: any) => pdb.spec?.maxUnavailable ?? '-';
export const pdbAllowedDisruptions = (pdb: any) => pdb.status?.disruptionsAllowed ?? '-';

/** Sort/filter semantics of the PodDisruptionBudget list columns. */
export const pdbSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    minAvailable: { getValue: pdbMinAvailable },
    maxUnavailable: { getValue: pdbMaxUnavailable },
    allowed: { getValue: pdbAllowedDisruptions },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
