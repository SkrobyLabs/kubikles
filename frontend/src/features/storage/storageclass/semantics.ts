import type { SemanticsMap } from '~/utils/columnSemantics';

export const STORAGE_DEFAULT_ANNOTATION = 'storageclass.kubernetes.io/is-default-class';

export function isDefaultStorageClass(storageClass: any): boolean {
    return storageClass.metadata?.annotations?.[STORAGE_DEFAULT_ANNOTATION] === 'true';
}

/** Sort/filter semantics of the storage class list columns. */
export const storageClassSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    provisioner: { getValue: (item: any) => item.provisioner || '' },
    reclaimPolicy: { getValue: (item: any) => item.reclaimPolicy || '' },
    volumeBindingMode: { getValue: (item: any) => item.volumeBindingMode || '' },
    allowVolumeExpansion: { getValue: (item: any) => item.allowVolumeExpansion ? 'Yes' : 'No' },
    /** Not shown as a column of its own: the name cell carries a default badge. */
    isDefault: { getValue: (item: any) => isDefaultStorageClass(item) ? 'Yes' : 'No', filterable: false },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
