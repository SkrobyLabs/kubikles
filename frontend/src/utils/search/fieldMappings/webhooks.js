/**
 * Webhook Configuration Field Mappings
 *
 * ValidatingWebhookConfiguration and MutatingWebhookConfiguration fields for advanced search filtering.
 */

import { commonFields } from './common';

const webhookBaseFields = {
    ...commonFields,

    webhooks: {
        extractor: (item) => String(item.webhooks?.length || 0),
        aliases: ['webhookcount', 'count']
    },

    failurepolicy: {
        extractor: (item) => {
            const policies = new Set();
            (item.webhooks || []).forEach(wh => {
                policies.add(wh.failurePolicy || 'Fail');
            });
            return Array.from(policies).join(' ');
        },
        aliases: ['failure', 'onfailure']
    },

    sideeffects: {
        extractor: (item) => {
            const effects = new Set();
            (item.webhooks || []).forEach(wh => {
                effects.add(wh.sideEffects || 'Unknown');
            });
            return Array.from(effects).join(' ');
        },
        aliases: ['sideeffect']
    },

    service: {
        extractor: (item) => {
            const services = [];
            (item.webhooks || []).forEach(wh => {
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
        extractor: (item) => {
            const resources = new Set();
            (item.webhooks || []).forEach(wh => {
                (wh.rules || []).forEach(rule => {
                    (rule.resources || []).forEach(r => resources.add(r));
                });
            });
            return Array.from(resources).join(' ');
        },
        aliases: ['resource']
    },

    operations: {
        extractor: (item) => {
            const ops = new Set();
            (item.webhooks || []).forEach(wh => {
                (wh.rules || []).forEach(rule => {
                    (rule.operations || []).forEach(op => ops.add(op));
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
        extractor: (item) => {
            const policies = new Set();
            (item.webhooks || []).forEach(wh => {
                policies.add(wh.reinvocationPolicy || 'Never');
            });
            return Array.from(policies).join(' ');
        },
        aliases: ['reinvocationpolicy']
    }
};
