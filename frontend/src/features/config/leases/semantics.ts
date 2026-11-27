import type { SemanticsMap } from '~/utils/columnSemantics';

export const leaseHolderIdentity = (lease: any) => lease.spec?.holderIdentity || '-';

/** Sort/filter semantics of the Lease list columns. */
export const leaseSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    holder: { getValue: leaseHolderIdentity },
    duration: { getValue: (item: any) => item.spec?.leaseDurationSeconds || 0 },
    renewTime: { getValue: (item: any) => item.spec?.renewTime || '' },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
