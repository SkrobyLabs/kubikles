import { describe, it, expect } from 'vitest';
import {
    getFieldsForResource,
    getFieldByName,
    getAvailableFieldNames,
    getFieldsMetadata
} from './index';
import { commonFields } from './common';
import { podFields } from './pods';

describe('getFieldsForResource', () => {
    it('returns pod fields for pods resource type', () => {
        const fields = getFieldsForResource('pods');
        expect(fields).toBe(podFields);
    });

    it('returns correct fields for each registered resource type', () => {
        const resourceTypes = [
            'pods', 'deployments', 'statefulsets', 'daemonsets',
            'replicasets', 'jobs', 'cronjobs', 'nodes', 'services',
            'secrets', 'configmaps', 'namespaces', 'events',
            'serviceaccounts', 'roles', 'clusterroles', 'rolebindings', 'clusterrolebindings'
        ];

        for (const type of resourceTypes) {
            const fields = getFieldsForResource(type);
            expect(fields).toBeDefined();
            expect(fields.name).toBeDefined(); // All should have common fields
        }
    });

    it('returns common fields for unknown resource type', () => {
        const fields = getFieldsForResource('unknowntype');
        expect(fields).toBe(commonFields);
    });

    it('returns common fields for undefined', () => {
        const fields = getFieldsForResource(undefined);
        expect(fields).toBe(commonFields);
    });
});

describe('getFieldByName', () => {
    describe('direct field lookup', () => {
        it('finds field by exact name', () => {
            const field = getFieldByName('pods', 'name');
            expect(field).toBeDefined();
            expect(field.extractor).toBeDefined();
        });

        it('finds pod-specific fields', () => {
            const nodename = getFieldByName('pods', 'nodename');
            expect(nodename).toBeDefined();

            const status = getFieldByName('pods', 'status');
            expect(status).toBeDefined();
        });

        it('is case-insensitive', () => {
            const field1 = getFieldByName('pods', 'NodeName');
            const field2 = getFieldByName('pods', 'NODENAME');
            const field3 = getFieldByName('pods', 'nodename');

            expect(field1).toBe(field2);
            expect(field2).toBe(field3);
        });

        it('returns null for non-existent field', () => {
            const field = getFieldByName('pods', 'nonexistent');
            expect(field).toBeNull();
        });
    });

    describe('alias lookup', () => {
        it('finds field by alias "n" -> name', () => {
            const field = getFieldByName('pods', 'n');
            expect(field).toBeDefined();
            expect(field).toBe(getFieldByName('pods', 'name'));
        });

        it('finds field by alias "ns" -> namespace', () => {
            const field = getFieldByName('pods', 'ns');
            expect(field).toBeDefined();
            expect(field).toBe(getFieldByName('pods', 'namespace'));
        });

        it('finds field by alias "node" -> nodename', () => {
            const field = getFieldByName('pods', 'node');
            expect(field).toBeDefined();
            expect(field).toBe(getFieldByName('pods', 'nodename'));
        });

        it('finds field by alias "s" -> status', () => {
            const field = getFieldByName('pods', 's');
            expect(field).toBeDefined();
            expect(field).toBe(getFieldByName('pods', 'status'));
        });

        it('finds field by alias "sa" -> serviceaccount', () => {
            const field = getFieldByName('pods', 'sa');
            expect(field).toBeDefined();
            expect(field).toBe(getFieldByName('pods', 'serviceaccount'));
        });

        it('finds labels by alias "l"', () => {
            const field = getFieldByName('pods', 'l');
            expect(field).toBeDefined();
            expect(field).toBe(getFieldByName('pods', 'labels'));
        });
    });

    describe('different resource types', () => {
        it('returns common fields for all types', () => {
            const types = ['pods', 'deployments', 'services', 'nodes'];
            for (const type of types) {
                expect(getFieldByName(type, 'name')).toBeDefined();
                expect(getFieldByName(type, 'namespace')).toBeDefined();
                expect(getFieldByName(type, 'labels')).toBeDefined();
            }
        });

        it('returns type-specific fields only for that type', () => {
            // nodename is pod-specific
            expect(getFieldByName('pods', 'nodename')).toBeDefined();
            expect(getFieldByName('services', 'nodename')).toBeNull();
        });
    });
});

describe('getAvailableFieldNames', () => {
    it('returns array of field names for pods', () => {
        const names = getAvailableFieldNames('pods');
        expect(Array.isArray(names)).toBe(true);
        expect(names.length).toBeGreaterThan(0);
    });

    it('includes primary field names', () => {
        const names = getAvailableFieldNames('pods');
        expect(names).toContain('name');
        expect(names).toContain('namespace');
        expect(names).toContain('nodename');
        expect(names).toContain('status');
    });

    it('includes aliases', () => {
        const names = getAvailableFieldNames('pods');
        expect(names).toContain('n'); // alias for name
        expect(names).toContain('ns'); // alias for namespace
        expect(names).toContain('node'); // alias for nodename
        expect(names).toContain('s'); // alias for status
    });

    it('returns common field names for unknown type', () => {
        const names = getAvailableFieldNames('unknowntype');
        expect(names).toContain('name');
        expect(names).toContain('namespace');
        expect(names).toContain('labels');
    });
});

describe('getFieldsMetadata', () => {
    it('returns array of field metadata objects', () => {
        const metadata = getFieldsMetadata('pods');
        expect(Array.isArray(metadata)).toBe(true);
        expect(metadata.length).toBeGreaterThan(0);
    });

    it('each metadata object has name and aliases', () => {
        const metadata = getFieldsMetadata('pods');
        for (const field of metadata) {
            expect(field).toHaveProperty('name');
            expect(field).toHaveProperty('aliases');
            expect(Array.isArray(field.aliases)).toBe(true);
        }
    });

    it('includes pod-specific fields', () => {
        const metadata = getFieldsMetadata('pods');
        const fieldNames = metadata.map(m => m.name);
        expect(fieldNames).toContain('nodename');
        expect(fieldNames).toContain('status');
        expect(fieldNames).toContain('restarts');
    });

    it('includes correct aliases in metadata', () => {
        const metadata = getFieldsMetadata('pods');
        const statusField = metadata.find(m => m.name === 'status');
        expect(statusField.aliases).toContain('phase');
        expect(statusField.aliases).toContain('s');
    });
});
