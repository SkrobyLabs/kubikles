import type { SemanticsMap } from '~/utils/columnSemantics';

export const ingressHosts = (ingress: any): string => {
    const rules = ingress.spec?.rules || [];
    const hosts = rules.map((r: any) => r.host).filter(Boolean);
    return hosts.length > 0 ? hosts.join(', ') : '*';
};

export const ingressPaths = (ingress: any): string => {
    const rules = ingress.spec?.rules || [];
    const paths = [];
    for (const rule of rules) {
        const httpPaths = rule.http?.paths || [];
        for (const p of httpPaths) {
            paths.push(p.path || '/');
        }
    }
    return paths.length > 0 ? paths.slice(0, 3).join(', ') + (paths.length > 3 ? '...' : '') : '-';
};

export const ingressClass = (ingress: any): string =>
    ingress.spec?.ingressClassName || ingress.metadata?.annotations?.['kubernetes.io/ingress.class'] || '-';

export const ingressAddress = (ingress: any): string => {
    const lbIngress = ingress.status?.loadBalancer?.ingress || [];
    if (lbIngress.length === 0) return '-';
    const addresses = lbIngress.map((lb: any) => lb.ip || lb.hostname).filter(Boolean);
    return addresses.length > 0 ? addresses.join(', ') : '-';
};

export const ingressStatus = (ingress: any): { status: string; color: string } => {
    const lbIngress = ingress.status?.loadBalancer?.ingress || [];
    const rules = ingress.spec?.rules || [];
    const defaultBackend = ingress.spec?.defaultBackend;

    if (lbIngress.length > 0) {
        const hasAddress = lbIngress.some((lb: any) => lb.ip || lb.hostname);
        if (hasAddress) return { status: 'Active', color: 'text-green-400' };
    }
    if (rules.length === 0 && !defaultBackend) {
        return { status: 'No Rules', color: 'text-red-400' };
    }
    return { status: 'Pending', color: 'text-yellow-400' };
};

export const ingressTlsCount = (ingress: any): number => (ingress.spec?.tls || []).length;

/**
 * What the default backend column shows: empty without a backend, the dash for a backend that is
 * not a service (a resource backend), else "service:port".
 */
export const ingressDefaultBackendTarget = (ingress: any): string => {
    const backend = ingress.spec?.defaultBackend;
    if (!backend) return '';
    const svc = backend.service;
    if (svc) return `${svc.name}:${svc.port?.number || svc.port?.name}`;
    return '-';
};

/** Sort/filter semantics of the Ingress list columns. */
export const ingressSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    status: { getValue: (item: any) => ingressStatus(item).status },
    class: { getValue: ingressClass },
    hosts: { getValue: ingressHosts },
    address: { getValue: ingressAddress },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
    paths: { getValue: ingressPaths },
    tls: { getValue: (item: any) => ingressTlsCount(item) > 0 ? 'Yes' : 'No' },
    /** Not shown as a column of its own: the TLS cell reads "Yes (n)" and needs the count. */
    tlsCount: { getValue: ingressTlsCount, filterable: false },
    defaultBackend: { getValue: (item: any) => item.spec?.defaultBackend?.service?.name || '' },
    /** Not shown as a column of its own: the default backend cell reads "service:port". */
    defaultBackendTarget: { getValue: ingressDefaultBackendTarget, filterable: false },
    rules: { getValue: (item: any) => (item.spec?.rules || []).length },
} satisfies SemanticsMap;
