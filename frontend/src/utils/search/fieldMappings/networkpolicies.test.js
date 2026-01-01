import { describe, it, expect } from 'vitest';
import { networkPolicyFields } from './networkpolicies';

describe('networkPolicyFields', () => {
    describe('podselector', () => {
        it('extracts pod selector labels', () => {
            const np = {
                spec: {
                    podSelector: {
                        matchLabels: { app: 'web', tier: 'frontend' }
                    }
                }
            };
            expect(networkPolicyFields.podselector.extractor(np)).toBe('app=web tier=frontend');
        });

        it('returns "all" for empty or missing selector', () => {
            expect(networkPolicyFields.podselector.extractor({ spec: { podSelector: {} } })).toBe('all');
            expect(networkPolicyFields.podselector.extractor({})).toBe('all');
        });
    });

    describe('policytype', () => {
        it('extracts policy types', () => {
            const np = { spec: { policyTypes: ['Ingress', 'Egress'] } };
            expect(networkPolicyFields.policytype.extractor(np)).toBe('Ingress Egress');
        });

        it('defaults to Ingress when not specified', () => {
            expect(networkPolicyFields.policytype.extractor({})).toBe('Ingress');
        });
    });

    describe('ingress/egress', () => {
        it('returns true/false based on rule existence', () => {
            const withRules = { spec: { ingress: [{}], egress: [{}] } };
            expect(networkPolicyFields.ingress.extractor(withRules)).toBe('true');
            expect(networkPolicyFields.egress.extractor(withRules)).toBe('true');

            expect(networkPolicyFields.ingress.extractor({})).toBe('false');
            expect(networkPolicyFields.egress.extractor({})).toBe('false');
        });
    });

    describe('ingresscount/egresscount', () => {
        it('counts rules correctly', () => {
            const np = { spec: { ingress: [{}, {}, {}], egress: [{}, {}] } };
            expect(networkPolicyFields.ingresscount.extractor(np)).toBe('3');
            expect(networkPolicyFields.egresscount.extractor(np)).toBe('2');
        });

        it('returns 0 for missing rules', () => {
            expect(networkPolicyFields.ingresscount.extractor({})).toBe('0');
            expect(networkPolicyFields.egresscount.extractor({})).toBe('0');
        });
    });
});
