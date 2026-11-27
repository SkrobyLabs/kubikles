import type { SemanticsMap } from '~/utils/columnSemantics';

export function getDriverCount(csiNode: any): number {
    const drivers = csiNode.spec?.drivers || [];
    return drivers.length;
}

export function getDriverNames(csiNode: any): string {
    const drivers = csiNode.spec?.drivers || [];
    if (drivers.length === 0) return '-';
    if (drivers.length <= 2) {
        return drivers.map((d: any) => d.name).join(', ');
    }
    return `${drivers[0].name}, +${drivers.length - 1} more`;
}

/** Sort/filter semantics of the CSI node list columns. */
export const csiNodeSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    driverCount: { getValue: (item: any) => getDriverCount(item) },
    driverNames: { getValue: (item: any) => getDriverNames(item) },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
