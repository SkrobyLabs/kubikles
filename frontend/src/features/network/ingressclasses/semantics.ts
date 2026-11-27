import type { SemanticsMap } from '~/utils/columnSemantics';

export const INGRESS_DEFAULT_ANNOTATION = 'ingressclass.kubernetes.io/is-default-class';

export function isDefaultIngressClass(ingressClass: any): boolean {
    return ingressClass.metadata?.annotations?.[INGRESS_DEFAULT_ANNOTATION] === 'true';
}

/** Sort/filter semantics of the ingress class list columns. */
export const ingressClassSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    controller: { getValue: (item: any) => item.spec?.controller || '' },
    default: { getValue: (item: any) => isDefaultIngressClass(item) ? 'Yes' : 'No' },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
