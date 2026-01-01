import { describe, it, expect } from 'vitest';
import { pdbFields } from './pdbs';

describe('pdbFields', () => {
    describe('budget constraints', () => {
        it('extracts minAvailable and maxUnavailable', () => {
            expect(pdbFields.minavailable.extractor({ spec: { minAvailable: 2 } })).toBe('2');
            expect(pdbFields.minavailable.extractor({ spec: { minAvailable: '50%' } })).toBe('50%');
            expect(pdbFields.maxunavailable.extractor({ spec: { maxUnavailable: 1 } })).toBe('1');
            expect(pdbFields.maxunavailable.extractor({ spec: { maxUnavailable: '25%' } })).toBe('25%');
        });

        it('returns empty for missing values', () => {
            expect(pdbFields.minavailable.extractor({})).toBe('');
            expect(pdbFields.maxunavailable.extractor({})).toBe('');
        });
    });

    describe('selector', () => {
        it('extracts selector labels', () => {
            const pdb = {
                spec: {
                    selector: { matchLabels: { app: 'web', version: 'v1' } }
                }
            };
            expect(pdbFields.selector.extractor(pdb)).toBe('app=web version=v1');
        });

        it('returns empty for missing selector', () => {
            expect(pdbFields.selector.extractor({})).toBe('');
        });
    });

    describe('status fields', () => {
        it('extracts status values', () => {
            const pdb = {
                status: {
                    currentHealthy: 3,
                    desiredHealthy: 2,
                    disruptionsAllowed: 1,
                    expectedPods: 5
                }
            };
            expect(pdbFields.currenthealthy.extractor(pdb)).toBe('3');
            expect(pdbFields.desiredhealthy.extractor(pdb)).toBe('2');
            expect(pdbFields.disruptionsallowed.extractor(pdb)).toBe('1');
            expect(pdbFields.expectedpods.extractor(pdb)).toBe('5');
        });

        it('returns empty for missing status', () => {
            expect(pdbFields.currenthealthy.extractor({})).toBe('');
            expect(pdbFields.desiredhealthy.extractor({})).toBe('');
            expect(pdbFields.disruptionsallowed.extractor({})).toBe('');
            expect(pdbFields.expectedpods.extractor({})).toBe('');
        });
    });
});
