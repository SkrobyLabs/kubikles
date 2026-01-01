import { describe, it, expect } from 'vitest';
import { createNamespaceKey } from './useResourceWatcher';

describe('createNamespaceKey', () => {
    describe('single namespace handling', () => {
        it('returns the namespace as-is for single string', () => {
            expect(createNamespaceKey('default')).toBe('default');
        });

        it('returns the namespace for single-element array', () => {
            expect(createNamespaceKey(['default'])).toBe('default');
        });

        it('handles empty string namespace (all namespaces)', () => {
            expect(createNamespaceKey('')).toBe('');
        });

        it('handles empty string in array', () => {
            expect(createNamespaceKey([''])).toBe('');
        });
    });

    describe('multiple namespaces handling', () => {
        it('joins multiple namespaces with comma', () => {
            expect(createNamespaceKey(['ns1', 'ns2'])).toBe('ns1,ns2');
        });

        it('sorts namespaces for stable key', () => {
            expect(createNamespaceKey(['ns2', 'ns1'])).toBe('ns1,ns2');
        });

        it('produces same key regardless of input order', () => {
            const key1 = createNamespaceKey(['alpha', 'beta', 'gamma']);
            const key2 = createNamespaceKey(['gamma', 'alpha', 'beta']);
            const key3 = createNamespaceKey(['beta', 'gamma', 'alpha']);

            expect(key1).toBe(key2);
            expect(key2).toBe(key3);
            expect(key1).toBe('alpha,beta,gamma');
        });

        it('handles many namespaces', () => {
            const namespaces = ['z', 'a', 'm', 'b', 'x'];
            expect(createNamespaceKey(namespaces)).toBe('a,b,m,x,z');
        });
    });

    describe('null/undefined handling', () => {
        it('returns empty string for null', () => {
            expect(createNamespaceKey(null)).toBe('');
        });

        it('returns empty string for undefined', () => {
            expect(createNamespaceKey(undefined)).toBe('');
        });
    });

    describe('does not mutate input', () => {
        it('does not modify the original array', () => {
            const original = ['ns2', 'ns1', 'ns3'];
            const copy = [...original];
            createNamespaceKey(original);
            expect(original).toEqual(copy);
        });
    });

    describe('real-world scenarios', () => {
        it('handles kube-system namespace', () => {
            expect(createNamespaceKey('kube-system')).toBe('kube-system');
        });

        it('handles typical multi-namespace selection', () => {
            const namespaces = ['production', 'staging', 'development'];
            expect(createNamespaceKey(namespaces)).toBe('development,production,staging');
        });

        it('handles namespaces with numbers', () => {
            const namespaces = ['app-2', 'app-1', 'app-10'];
            // Note: lexicographic sort, so app-10 comes before app-2
            expect(createNamespaceKey(namespaces)).toBe('app-1,app-10,app-2');
        });

        it('handles duplicate namespaces in input', () => {
            const namespaces = ['ns1', 'ns2', 'ns1'];
            // Duplicates are preserved (caller should dedupe if needed)
            expect(createNamespaceKey(namespaces)).toBe('ns1,ns1,ns2');
        });
    });
});
