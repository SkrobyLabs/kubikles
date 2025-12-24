import { describe, it, expect } from 'vitest';
import { serviceAccountFields } from './serviceaccounts';

describe('serviceAccountFields', () => {
    describe('common fields inheritance', () => {
        it('includes name field from common', () => {
            expect(serviceAccountFields.name).toBeDefined();
            expect(serviceAccountFields.name.extractor({ metadata: { name: 'default' } })).toBe('default');
        });

        it('includes namespace field from common', () => {
            expect(serviceAccountFields.namespace).toBeDefined();
            expect(serviceAccountFields.namespace.extractor({ metadata: { namespace: 'kube-system' } })).toBe('kube-system');
        });

        it('includes labels field from common', () => {
            expect(serviceAccountFields.labels).toBeDefined();
        });
    });

    describe('secrets field', () => {
        it('extracts secret names', () => {
            const sa = {
                secrets: [
                    { name: 'default-token-abc' },
                    { name: 'my-secret' }
                ]
            };
            expect(serviceAccountFields.secrets.extractor(sa)).toBe('default-token-abc my-secret');
        });

        it('handles empty secrets array', () => {
            expect(serviceAccountFields.secrets.extractor({ secrets: [] })).toBe('');
        });

        it('handles missing secrets', () => {
            expect(serviceAccountFields.secrets.extractor({})).toBe('');
        });

        it('has correct aliases', () => {
            expect(serviceAccountFields.secrets.aliases).toContain('secret');
        });
    });

    describe('secretcount field', () => {
        it('counts secrets correctly', () => {
            const sa = { secrets: [{ name: 'a' }, { name: 'b' }, { name: 'c' }] };
            expect(serviceAccountFields.secretcount.extractor(sa)).toBe('3');
        });

        it('returns 0 for empty secrets', () => {
            expect(serviceAccountFields.secretcount.extractor({ secrets: [] })).toBe('0');
        });

        it('returns 0 for missing secrets', () => {
            expect(serviceAccountFields.secretcount.extractor({})).toBe('0');
        });

        it('has correct aliases', () => {
            expect(serviceAccountFields.secretcount.aliases).toContain('secretscount');
            expect(serviceAccountFields.secretcount.aliases).toContain('numsecrets');
        });
    });

    describe('imagepullsecrets field', () => {
        it('extracts image pull secret names', () => {
            const sa = {
                imagePullSecrets: [
                    { name: 'docker-registry' },
                    { name: 'gcr-secret' }
                ]
            };
            expect(serviceAccountFields.imagepullsecrets.extractor(sa)).toBe('docker-registry gcr-secret');
        });

        it('handles empty imagePullSecrets', () => {
            expect(serviceAccountFields.imagepullsecrets.extractor({ imagePullSecrets: [] })).toBe('');
        });

        it('handles missing imagePullSecrets', () => {
            expect(serviceAccountFields.imagepullsecrets.extractor({})).toBe('');
        });

        it('has correct aliases', () => {
            expect(serviceAccountFields.imagepullsecrets.aliases).toContain('pullsecrets');
            expect(serviceAccountFields.imagepullsecrets.aliases).toContain('imagepull');
        });
    });

    describe('automount field', () => {
        it('returns true when automountServiceAccountToken is true', () => {
            expect(serviceAccountFields.automount.extractor({ automountServiceAccountToken: true })).toBe('true');
        });

        it('returns false when automountServiceAccountToken is false', () => {
            expect(serviceAccountFields.automount.extractor({ automountServiceAccountToken: false })).toBe('false');
        });

        it('returns true when automountServiceAccountToken is undefined (default)', () => {
            expect(serviceAccountFields.automount.extractor({})).toBe('true');
        });

        it('has correct aliases', () => {
            expect(serviceAccountFields.automount.aliases).toContain('automounttoken');
            expect(serviceAccountFields.automount.aliases).toContain('automountserviceaccounttoken');
        });
    });
});
