import { describe, it, expect } from 'vitest';
import { commonFields } from './common';

describe('commonFields extractors', () => {
    describe('name', () => {
        it('extracts metadata.name', () => {
            const item = { metadata: { name: 'my-resource' } };
            expect(commonFields.name.extractor(item)).toBe('my-resource');
        });

        it('returns empty string when name is missing', () => {
            expect(commonFields.name.extractor({ metadata: {} })).toBe('');
            expect(commonFields.name.extractor({})).toBe('');
        });

        it('has alias "n"', () => {
            expect(commonFields.name.aliases).toContain('n');
        });
    });

    describe('namespace', () => {
        it('extracts metadata.namespace', () => {
            const item = { metadata: { namespace: 'kube-system' } };
            expect(commonFields.namespace.extractor(item)).toBe('kube-system');
        });

        it('returns empty string when namespace is missing', () => {
            expect(commonFields.namespace.extractor({ metadata: {} })).toBe('');
            expect(commonFields.namespace.extractor({})).toBe('');
        });

        it('has alias "ns"', () => {
            expect(commonFields.namespace.aliases).toContain('ns');
        });
    });

    describe('labels', () => {
        it('extracts and formats labels as key=value pairs', () => {
            const item = {
                metadata: {
                    labels: { app: 'nginx', version: 'v1' }
                }
            };
            const result = commonFields.labels.extractor(item);
            expect(result).toContain('app=nginx');
            expect(result).toContain('version=v1');
        });

        it('joins multiple labels with space', () => {
            const item = {
                metadata: {
                    labels: { a: '1', b: '2' }
                }
            };
            const result = commonFields.labels.extractor(item);
            expect(result.split(' ')).toHaveLength(2);
        });

        it('returns empty string when labels are missing', () => {
            expect(commonFields.labels.extractor({ metadata: {} })).toBe('');
            expect(commonFields.labels.extractor({})).toBe('');
        });

        it('handles empty labels object', () => {
            const item = { metadata: { labels: {} } };
            expect(commonFields.labels.extractor(item)).toBe('');
        });

        it('has aliases "label" and "l"', () => {
            expect(commonFields.labels.aliases).toContain('label');
            expect(commonFields.labels.aliases).toContain('l');
        });
    });

    describe('annotations', () => {
        it('extracts and formats annotations as key=value pairs', () => {
            const item = {
                metadata: {
                    annotations: {
                        'kubernetes.io/description': 'test resource',
                        'app.kubernetes.io/version': '1.0'
                    }
                }
            };
            const result = commonFields.annotations.extractor(item);
            expect(result).toContain('kubernetes.io/description=test resource');
            expect(result).toContain('app.kubernetes.io/version=1.0');
        });

        it('returns empty string when annotations are missing', () => {
            expect(commonFields.annotations.extractor({ metadata: {} })).toBe('');
            expect(commonFields.annotations.extractor({})).toBe('');
        });

        it('has aliases "annotation" and "a"', () => {
            expect(commonFields.annotations.aliases).toContain('annotation');
            expect(commonFields.annotations.aliases).toContain('a');
        });
    });

    describe('uid', () => {
        it('extracts metadata.uid', () => {
            const item = { metadata: { uid: 'abc-123-def-456' } };
            expect(commonFields.uid.extractor(item)).toBe('abc-123-def-456');
        });

        it('returns empty string when uid is missing', () => {
            expect(commonFields.uid.extractor({ metadata: {} })).toBe('');
            expect(commonFields.uid.extractor({})).toBe('');
        });

        it('has no aliases', () => {
            expect(commonFields.uid.aliases).toHaveLength(0);
        });
    });
});
