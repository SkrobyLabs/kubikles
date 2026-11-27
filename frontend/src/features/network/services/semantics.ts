import type { SemanticsMap } from '~/utils/columnSemantics';

export const servicePortsDisplay = (item: any): string => item.spec?.ports?.map((p: any) => `${p.port}/${p.protocol}`).join(', ') || '';

/** Load balancer addresses (IP, else hostname) of a service, empty entries dropped. */
export const serviceLoadBalancerAddresses = (item: any): string[] =>
    item.status?.loadBalancer?.ingress?.map((i: any) => i.ip || i.hostname).filter(Boolean) || [];

export const serviceSelectorText = (item: any): string =>
    Object.entries(item.spec?.selector || {}).map(([k, v]) => `${k}=${v}`).join(',');

/** Sort/filter semantics of the Service list columns. */
export const serviceSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    type: { getValue: (item: any) => item.spec?.type },
    clusterIP: { getValue: (item: any) => item.spec?.clusterIP },
    ports: { getValue: servicePortsDisplay },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
    externalIP: { getValue: (item: any) => item.status?.loadBalancer?.ingress?.[0]?.ip || '' },
    /** Not shown as a column of its own: the external IP cell lists every address (IP or hostname). */
    externalAddresses: { getValue: (item: any) => serviceLoadBalancerAddresses(item).join(', '), filterable: false },
    selector: { getValue: serviceSelectorText },
    sessionAffinity: { getValue: (item: any) => item.spec?.sessionAffinity || 'None' },
    externalName: { getValue: (item: any) => item.spec?.externalName || '' },
} satisfies SemanticsMap;
