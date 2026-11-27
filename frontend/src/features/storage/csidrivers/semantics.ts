import type { SemanticsMap } from '~/utils/columnSemantics';

export const getAttachRequired = (driver: any): boolean => driver.spec?.attachRequired ?? true;
export const getPodInfoOnMount = (driver: any): boolean => driver.spec?.podInfoOnMount ?? false;
export const getStorageCapacity = (driver: any): boolean => driver.spec?.storageCapacity ?? false;

export function getVolumeModes(driver: any): string {
    const modes = driver.spec?.volumeLifecycleModes || [];
    return modes.length > 0 ? modes.join(', ') : '-';
}

/** Sort/filter semantics of the CSI driver list columns. */
export const csiDriverSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    attachRequired: { getValue: (item: any) => getAttachRequired(item) ? 'Yes' : 'No' },
    podInfoOnMount: { getValue: (item: any) => getPodInfoOnMount(item) ? 'Yes' : 'No' },
    storageCapacity: { getValue: (item: any) => getStorageCapacity(item) ? 'Yes' : 'No' },
    volumeModes: { getValue: (item: any) => getVolumeModes(item) },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
