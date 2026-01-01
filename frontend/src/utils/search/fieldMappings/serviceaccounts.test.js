import { describe, it, expect } from 'vitest';
import { serviceAccountFields } from './serviceaccounts';

describe('serviceAccountFields', () => {
    describe('secrets', () => {
        it('extracts secret names', () => {
            const sa = {
                secrets: [{ name: 'default-token-abc' }, { name: 'my-secret' }]
            };
            expect(serviceAccountFields.secrets.extractor(sa)).toBe('default-token-abc my-secret');
        });

        it('handles empty or missing secrets', () => {
            expect(serviceAccountFields.secrets.extractor({ secrets: [] })).toBe('');
            expect(serviceAccountFields.secrets.extractor({})).toBe('');
        });
    });

    describe('secretcount', () => {
        it('counts secrets correctly', () => {
            expect(serviceAccountFields.secretcount.extractor({ secrets: [{}, {}, {}] })).toBe('3');
            expect(serviceAccountFields.secretcount.extractor({})).toBe('0');
        });
    });

    describe('imagepullsecrets', () => {
        it('extracts image pull secret names', () => {
            const sa = {
                imagePullSecrets: [{ name: 'docker-registry' }, { name: 'gcr-secret' }]
            };
            expect(serviceAccountFields.imagepullsecrets.extractor(sa)).toBe('docker-registry gcr-secret');
        });

        it('handles empty or missing imagePullSecrets', () => {
            expect(serviceAccountFields.imagepullsecrets.extractor({ imagePullSecrets: [] })).toBe('');
            expect(serviceAccountFields.imagepullsecrets.extractor({})).toBe('');
        });
    });

    describe('automount', () => {
        it('returns token automount status', () => {
            expect(serviceAccountFields.automount.extractor({ automountServiceAccountToken: true })).toBe('true');
            expect(serviceAccountFields.automount.extractor({ automountServiceAccountToken: false })).toBe('false');
            expect(serviceAccountFields.automount.extractor({})).toBe('true'); // default
        });
    });
});
