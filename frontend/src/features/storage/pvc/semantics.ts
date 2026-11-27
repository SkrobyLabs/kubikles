import type { SemanticsMap } from '~/utils/columnSemantics';

/** Capacity as the list shows it: the bound capacity, else the requested size, else ''. */
export const pvcCapacity = (item: any): string => item.status?.capacity?.storage || item.spec?.resources?.requests?.storage || '';

/** Sort/filter semantics of the PersistentVolumeClaim list columns. */
export const pvcSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    status: { getValue: (item: any) => item.status?.phase },
    volume: { getValue: (item: any) => item.spec?.volumeName || '' },
    capacity: { getValue: pvcCapacity },
    accessModes: { getValue: (item: any) => item.spec?.accessModes?.join(', ') || '' },
    storageClass: { getValue: (item: any) => item.spec?.storageClassName || '' },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
    volumeMode: { getValue: (item: any) => item.spec?.volumeMode || 'Filesystem' },
    requested: { getValue: (item: any) => item.spec?.resources?.requests?.storage || '' },
    selector: { getValue: (item: any) => Object.entries(item.spec?.selector?.matchLabels || {}).map(([k, v]) => `${k}=${v}`).join(',') },
} satisfies SemanticsMap;
