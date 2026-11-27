import { describe, expect, it } from 'vitest';
import { decideAvailability } from './availability';

const info = { enabled: true, types: ['configmaps', 'events'] };

describe('decideAvailability', () => {
    const none = new Set<string>();

    it('is off whenever the setting is off, even if the backend would serve the type', () => {
        expect(decideAvailability('configmaps', false, info, none)).toBe('off');
    });

    it('is off for types without a generated schema, without waiting for the backend', () => {
        expect(decideAvailability('cronjobs', true, null, none)).toBe('off');
    });

    it('is pending until the backend answered', () => {
        expect(decideAvailability('configmaps', true, null, none)).toBe('pending');
    });

    it('is off when the backend disabled views (kill switch) or does not register the type', () => {
        expect(decideAvailability('configmaps', true, { enabled: false, types: ['configmaps'] }, none)).toBe('off');
        expect(decideAvailability('secrets', true, info, none)).toBe('off');
    });

    it('is off for a kind that failed repeatedly in this session', () => {
        expect(decideAvailability('configmaps', true, info, new Set(['configmaps']))).toBe('off');
        expect(decideAvailability('events', true, info, new Set(['configmaps']))).toBe('on');
    });

    it('is on when setting, backend and schema agree', () => {
        expect(decideAvailability('configmaps', true, info, none)).toBe('on');
    });

    it('serves custom resource types dynamically when the backend says so', () => {
        const crd = 'crd:cert.io/v1/certs';
        expect(decideAvailability(crd, true, null, none)).toBe('pending');
        expect(decideAvailability(crd, true, { enabled: true, types: [], dynamic: true }, none)).toBe('on');
        expect(decideAvailability(crd, true, { enabled: true, types: [] }, none)).toBe('off');
        expect(decideAvailability(crd, true, { enabled: false, types: [], dynamic: true }, none)).toBe('off');
        expect(decideAvailability(crd, false, { enabled: true, types: [], dynamic: true }, none)).toBe('off');
        expect(decideAvailability(crd, true, { enabled: true, types: [], dynamic: true }, new Set([crd]))).toBe('off');
    });
});
