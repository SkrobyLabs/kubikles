import type { SemanticsMap } from '~/utils/columnSemantics';

export const endpointsAddressCount = (ep: any): number => {
    let count = 0;
    (ep.subsets || []).forEach((subset: any) => {
        count += (subset.addresses || []).length;
        count += (subset.notReadyAddresses || []).length;
    });
    return count;
};

export const endpointsReadyCount = (ep: any): number => {
    let count = 0;
    (ep.subsets || []).forEach((subset: any) => {
        count += (subset.addresses || []).length;
    });
    return count;
};

export const endpointsPorts = (ep: any): string => {
    const ports = new Set<any>();
    (ep.subsets || []).forEach((subset: any) => {
        (subset.ports || []).forEach((port: any) => {
            ports.add(`${port.port}/${port.protocol || 'TCP'}`);
        });
    });
    return ports.size > 0 ? Array.from(ports).slice(0, 3).join(', ') + (ports.size > 3 ? ` +${ports.size - 3}` : '') : '-';
};

/** Sort/filter semantics of the Endpoints list columns. */
export const endpointsSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    endpoints: { getValue: endpointsAddressCount },
    ports: { getValue: endpointsPorts },
    /** Not shown as a column of its own: the endpoints cell reads "ready/total" and needs the ready half. */
    readyCount: { getValue: endpointsReadyCount, filterable: false },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
