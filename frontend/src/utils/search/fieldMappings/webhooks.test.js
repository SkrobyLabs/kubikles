import { describe, it, expect } from 'vitest';
import { validatingWebhookFields, mutatingWebhookFields } from './webhooks';

describe('validatingWebhookFields', () => {
    describe('webhooks', () => {
        it('counts webhooks correctly', () => {
            expect(validatingWebhookFields.webhooks.extractor({ webhooks: [{}, {}, {}] })).toBe('3');
            expect(validatingWebhookFields.webhooks.extractor({})).toBe('0');
        });
    });

    describe('failurepolicy', () => {
        it('extracts failure policies', () => {
            const config = {
                webhooks: [
                    { failurePolicy: 'Fail' },
                    { failurePolicy: 'Ignore' }
                ]
            };
            const result = validatingWebhookFields.failurepolicy.extractor(config);
            expect(result).toContain('Fail');
            expect(result).toContain('Ignore');
        });

        it('defaults to Fail', () => {
            const config = { webhooks: [{}] };
            expect(validatingWebhookFields.failurepolicy.extractor(config)).toBe('Fail');
        });
    });

    describe('sideeffects', () => {
        it('extracts side effects', () => {
            const config = {
                webhooks: [
                    { sideEffects: 'None' },
                    { sideEffects: 'NoneOnDryRun' }
                ]
            };
            const result = validatingWebhookFields.sideeffects.extractor(config);
            expect(result).toContain('None');
            expect(result).toContain('NoneOnDryRun');
        });
    });

    describe('service', () => {
        it('extracts service targets', () => {
            const config = {
                webhooks: [{
                    clientConfig: {
                        service: { namespace: 'kube-system', name: 'webhook-svc' }
                    }
                }]
            };
            expect(validatingWebhookFields.service.extractor(config)).toBe('kube-system/webhook-svc');
        });
    });

    describe('resources', () => {
        it('extracts resources from rules', () => {
            const config = {
                webhooks: [{
                    rules: [
                        { resources: ['pods', 'services'] },
                        { resources: ['deployments'] }
                    ]
                }]
            };
            const result = validatingWebhookFields.resources.extractor(config);
            expect(result).toContain('pods');
            expect(result).toContain('services');
            expect(result).toContain('deployments');
        });
    });

    describe('operations', () => {
        it('extracts operations from rules', () => {
            const config = {
                webhooks: [{
                    rules: [
                        { operations: ['CREATE', 'UPDATE'] },
                        { operations: ['DELETE'] }
                    ]
                }]
            };
            const result = validatingWebhookFields.operations.extractor(config);
            expect(result).toContain('CREATE');
            expect(result).toContain('UPDATE');
            expect(result).toContain('DELETE');
        });
    });
});

describe('mutatingWebhookFields', () => {
    it('inherits validating webhook fields', () => {
        expect(mutatingWebhookFields.webhooks).toBeDefined();
        expect(mutatingWebhookFields.failurepolicy).toBeDefined();
    });

    describe('reinvocation', () => {
        it('extracts reinvocation policy', () => {
            const config = {
                webhooks: [
                    { reinvocationPolicy: 'IfNeeded' },
                    { reinvocationPolicy: 'Never' }
                ]
            };
            const result = mutatingWebhookFields.reinvocation.extractor(config);
            expect(result).toContain('IfNeeded');
            expect(result).toContain('Never');
        });

        it('defaults to Never', () => {
            const config = { webhooks: [{}] };
            expect(mutatingWebhookFields.reinvocation.extractor(config)).toBe('Never');
        });
    });
});
