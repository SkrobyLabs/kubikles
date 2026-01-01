import { describe, it, expect } from 'vitest';
import { limitRangeFields } from './limitranges';

describe('limitRangeFields', () => {
    describe('limittype', () => {
        it('extracts limit types', () => {
            const lr = {
                spec: {
                    limits: [{ type: 'Container' }, { type: 'Pod' }]
                }
            };
            expect(limitRangeFields.limittype.extractor(lr)).toBe('Container Pod');
        });

        it('returns empty for missing limits', () => {
            expect(limitRangeFields.limittype.extractor({})).toBe('');
        });
    });

    describe('limitcount', () => {
        it('counts limits correctly', () => {
            const lr = { spec: { limits: [{}, {}, {}] } };
            expect(limitRangeFields.limitcount.extractor(lr)).toBe('3');
        });

        it('returns 0 for missing limits', () => {
            expect(limitRangeFields.limitcount.extractor({})).toBe('0');
        });
    });

    describe('type existence checks', () => {
        const withAllTypes = {
            spec: {
                limits: [
                    { type: 'Container' },
                    { type: 'Pod' },
                    { type: 'PersistentVolumeClaim' }
                ]
            }
        };
        const withOnlyContainer = { spec: { limits: [{ type: 'Container' }] } };

        it('detects Container type', () => {
            expect(limitRangeFields.hascontainer.extractor(withAllTypes)).toBe('true');
            expect(limitRangeFields.hascontainer.extractor({})).toBe('false');
        });

        it('detects Pod type', () => {
            expect(limitRangeFields.haspod.extractor(withAllTypes)).toBe('true');
            expect(limitRangeFields.haspod.extractor(withOnlyContainer)).toBe('false');
        });

        it('detects PVC type', () => {
            expect(limitRangeFields.haspvc.extractor(withAllTypes)).toBe('true');
            expect(limitRangeFields.haspvc.extractor(withOnlyContainer)).toBe('false');
        });
    });
});
