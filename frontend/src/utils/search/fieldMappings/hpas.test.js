import { describe, it, expect } from 'vitest';
import { hpaFields } from './hpas';

describe('hpaFields', () => {
    describe('target fields', () => {
        const hpa = {
            spec: {
                scaleTargetRef: { kind: 'Deployment', name: 'web-app' }
            }
        };

        it('extracts scale target reference', () => {
            expect(hpaFields.target.extractor(hpa)).toBe('Deployment/web-app');
            expect(hpaFields.targetkind.extractor(hpa)).toBe('Deployment');
            expect(hpaFields.targetname.extractor(hpa)).toBe('web-app');
        });

        it('handles missing scaleTargetRef', () => {
            expect(hpaFields.target.extractor({})).toBe('');
            expect(hpaFields.targetkind.extractor({})).toBe('');
            expect(hpaFields.targetname.extractor({})).toBe('');
        });
    });

    describe('replica fields', () => {
        it('extracts replica counts', () => {
            const hpa = {
                spec: { minReplicas: 2, maxReplicas: 10 },
                status: { currentReplicas: 5, desiredReplicas: 8 }
            };
            expect(hpaFields.minreplicas.extractor(hpa)).toBe('2');
            expect(hpaFields.maxreplicas.extractor(hpa)).toBe('10');
            expect(hpaFields.currentreplicas.extractor(hpa)).toBe('5');
            expect(hpaFields.desiredreplicas.extractor(hpa)).toBe('8');
        });

        it('defaults minReplicas to 1, others to empty', () => {
            expect(hpaFields.minreplicas.extractor({})).toBe('1');
            expect(hpaFields.maxreplicas.extractor({})).toBe('');
            expect(hpaFields.currentreplicas.extractor({})).toBe('');
            expect(hpaFields.desiredreplicas.extractor({})).toBe('');
        });
    });

    describe('metrics', () => {
        it('extracts metric types', () => {
            const hpa = {
                spec: {
                    metrics: [{ type: 'Resource' }, { type: 'Pods' }]
                }
            };
            expect(hpaFields.metrics.extractor(hpa)).toBe('Resource Pods');
        });

        it('returns empty for missing metrics', () => {
            expect(hpaFields.metrics.extractor({})).toBe('');
        });
    });
});
