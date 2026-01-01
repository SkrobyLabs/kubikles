import { describe, it, expect } from 'vitest';
import { resourceQuotaFields } from './resourcequotas';

describe('resourceQuotaFields', () => {
    describe('hard', () => {
        it('extracts hard limits', () => {
            const quota = {
                spec: { hard: { cpu: '4', memory: '8Gi', pods: '10' } }
            };
            const result = resourceQuotaFields.hard.extractor(quota);
            expect(result).toContain('cpu=4');
            expect(result).toContain('memory=8Gi');
            expect(result).toContain('pods=10');
        });

        it('returns empty for missing spec', () => {
            expect(resourceQuotaFields.hard.extractor({})).toBe('');
        });
    });

    describe('used', () => {
        it('extracts used resources from status', () => {
            const quota = {
                status: { used: { cpu: '2', memory: '4Gi' } }
            };
            const result = resourceQuotaFields.used.extractor(quota);
            expect(result).toContain('cpu=2');
            expect(result).toContain('memory=4Gi');
        });

        it('returns empty for missing status', () => {
            expect(resourceQuotaFields.used.extractor({})).toBe('');
        });
    });

    describe('scopes', () => {
        it('extracts scopes', () => {
            const quota = { spec: { scopes: ['NotTerminating', 'NotBestEffort'] } };
            expect(resourceQuotaFields.scopes.extractor(quota)).toBe('NotTerminating NotBestEffort');
        });

        it('returns empty for missing scopes', () => {
            expect(resourceQuotaFields.scopes.extractor({})).toBe('');
        });
    });

    describe('resourcecount', () => {
        it('counts hard limit resources', () => {
            const quota = { spec: { hard: { cpu: '4', memory: '8Gi', pods: '10' } } };
            expect(resourceQuotaFields.resourcecount.extractor(quota)).toBe('3');
        });

        it('returns 0 for missing spec', () => {
            expect(resourceQuotaFields.resourcecount.extractor({})).toBe('0');
        });
    });
});
