/**
 * Webhook Configuration Field Mappings
 *
 * ValidatingWebhookConfiguration and MutatingWebhookConfiguration fields for advanced search filtering.
 */

import { commonFields } from './common';

const webhookBaseFields = {
    ...commonFields,

    webhooks: {
        extractor: (item: any) => String(item.webhooks?.length || 0),
        aliases: ['webhookcount', 'count']
    },

    failurepolicy: {
        extractor: (item: any) => {
            const policies = new Set<any>();
            (item.webhooks || []).forEach((wh: any) => {
                policies.add(wh.failurePolicy || 'Fail');
            });
            return Array.from(policies).join(' ');
        },
        aliases: ['failure', 'onfailure']
    },

    sideeffects: {
        extractor: (item: any) => {
            const effects = new Set<any>();
            (item.webhooks || []).forEach((wh: any) => {
                effects.add(wh.sideEffects || 'Unknown');
            });
            return Array.from(effects).join(' ');
        },
        aliases: ['sideeffect']
    },

    service: {
        extractor: (item: any) => {
            const services: string[] = [];
            (item.webhooks || []).forEach((wh: any) => {
                if (wh.clientConfig?.service) {
                    const svc = wh.clientConfig.service;
                    services.push(`${svc.namespace}/${svc.name}`);
                }
            });
            return [...new Set(services)].join(' ');
        },
        aliases: ['services', 'target']
    },

    resources: {
        extractor: (item: any) => {
            const resources = new Set<any>();
            (item.webhooks || []).forEach((wh: any) => {
                (wh.rules || []).forEach((rule: any) => {
                    (rule.resources || []).forEach((r: any) => resources.add(r));
                });
            });
            return Array.from(resources).join(' ');
        },
        aliases: ['resource']
    },

    operations: {
        extractor: (item: any) => {
            const ops = new Set<any>();
            (item.webhooks || []).forEach((wh: any) => {
                (wh.rules || []).forEach((rule: any) => {
                    (rule.operations || []).forEach((op: any) => ops.add(op));
                });
            });
            return Array.from(ops).join(' ');
        },
        aliases: ['operation', 'ops']
    }
};

export const validatingWebhookFields = {
    ...webhookBaseFields
};

export const mutatingWebhookFields = {
    ...webhookBaseFields,

    reinvocation: {
        extractor: (item: any) => {
            const policies = new Set<any>();
            (item.webhooks || []).forEach((wh: any) => {
                policies.add(wh.reinvocationPolicy || 'Never');
            });
            return Array.from(policies).join(' ');
        },
        aliases: ['reinvocationpolicy']
    }
};
