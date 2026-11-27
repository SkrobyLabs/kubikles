import type { SemanticsMap } from '~/utils/columnSemantics';

export function getWebhookCount(config: any): number {
    return (config.webhooks || []).length;
}

export function getFailurePolicy(config: any): string {
    const policies = new Set<any>();
    (config.webhooks || []).forEach((wh: any) => {
        policies.add(wh.failurePolicy || 'Fail');
    });
    return Array.from(policies).join(', ') || '-';
}

export function getReinvocationPolicy(config: any): string {
    const policies = new Set<any>();
    (config.webhooks || []).forEach((wh: any) => {
        policies.add(wh.reinvocationPolicy || 'Never');
    });
    return Array.from(policies).join(', ') || '-';
}

/** Sort/filter semantics of the validating webhook configuration list columns. */
export const validatingWebhookSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    webhooks: { getValue: (item: any) => getWebhookCount(item) },
    failurePolicy: { getValue: (item: any) => getFailurePolicy(item) },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;

/** Sort/filter semantics of the mutating webhook configuration list columns. */
export const mutatingWebhookSemantics = {
    name: { getValue: (item: any) => item.metadata?.name },
    webhooks: { getValue: (item: any) => getWebhookCount(item) },
    failurePolicy: { getValue: (item: any) => getFailurePolicy(item) },
    reinvocation: { getValue: (item: any) => getReinvocationPolicy(item) },
    age: { getValue: (item: any) => item.metadata?.creationTimestamp },
} satisfies SemanticsMap;
