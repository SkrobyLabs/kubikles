import type { SemanticsMap } from '~/utils/columnSemantics';

export const endpointSliceEndpointCount = (eps: any): number => (eps.endpoints || []).length;

export const endpointSliceReadyCount = (eps: any): number =>
    (eps.endpoints || []).filter((ep: any) => ep.conditions?.ready === true).length;

export const endpointSlicePorts = (eps: any): string => {
    const ports = eps.ports || [];
    if (ports.length === 0) return '-';
    return ports.slice(0, 3).map((p: any) => `${p.port}/${p.protocol || 'TCP'}`).join(', ') +
        (ports.length > 3 ? ` +${ports.length - 3}` : '');
};

export const endpointSliceServiceName = (eps: any): string =>
    eps.metadata?.labels?.['kubernetes.io/service-name'] || '-';

/** Sort/filter semantics of the EndpointSlice list columns. */
export const endpointSliceSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    service: { getValue: endpointSliceServiceName },
    addressType: { getValue: (item: any) => item.addressType || '' },
    endpoints: { getValue: endpointSliceEndpointCount },
    ports: { getValue: endpointSlicePorts },
    /** Not shown as a column of its own: the endpoints cell reads "ready/total" and needs the ready half. */
    readyCount: { getValue: endpointSliceReadyCount, filterable: false },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
