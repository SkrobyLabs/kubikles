import type { SemanticsMap } from '~/utils/columnSemantics';

export const networkPolicyPodSelector = (policy: any) => {
    const selector = policy.spec?.podSelector;
    if (!selector || Object.keys(selector.matchLabels || {}).length === 0) {
        return 'All Pods';
    }
    const labels = Object.entries(selector.matchLabels || {})
        .map(([k, v]) => `${k}=${v}`)
        .slice(0, 2);
    const extra = Object.keys(selector.matchLabels || {}).length - 2;
    return labels.join(', ') + (extra > 0 ? `, +${extra}` : '');
};

export const networkPolicyTypes = (policy: any) => policy.spec?.policyTypes?.join(', ') || 'Ingress';

/** Sort/filter semantics of the NetworkPolicy list columns. */
export const networkPolicySemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    namespace: { getValue: (item: any) => item.metadata?.namespace },
    podSelector: { getValue: networkPolicyPodSelector },
    policyTypes: { getValue: networkPolicyTypes },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
