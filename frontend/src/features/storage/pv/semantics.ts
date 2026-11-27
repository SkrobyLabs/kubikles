import type { SemanticsMap } from '~/utils/columnSemantics';

export function getClaim(pv: any): string {
    return pv.spec?.claimRef ? `${pv.spec.claimRef.namespace}/${pv.spec.claimRef.name}` : '';
}

/** Sort/filter semantics of the persistent volume list columns. */
export const pvSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    capacity: { getValue: (item: any) => item.spec?.capacity?.storage || '' },
    accessModes: { getValue: (item: any) => item.spec?.accessModes?.join(', ') || '' },
    reclaimPolicy: { getValue: (item: any) => item.spec?.persistentVolumeReclaimPolicy || '' },
    status: { getValue: (item: any) => item.status?.phase },
    claim: { getValue: (item: any) => getClaim(item) },
    storageClass: { getValue: (item: any) => item.spec?.storageClassName || '' },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
