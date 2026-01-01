import { describe, it, expect } from 'vitest';
import { roleFields } from './roles';
import { clusterRoleFields } from './clusterroles';

describe('roleFields', () => {
    describe('rules', () => {
        it('extracts rule information', () => {
            const role = {
                rules: [
                    { apiGroups: [''], resources: ['pods'], verbs: ['get', 'list'] },
                    { apiGroups: ['apps'], resources: ['deployments'], verbs: ['*'] }
                ]
            };
            const result = roleFields.rules.extractor(role);
            expect(result).toContain('/pods:get,list');
            expect(result).toContain('apps/deployments:*');
        });

        it('handles empty or missing rules', () => {
            expect(roleFields.rules.extractor({ rules: [] })).toBe('');
            expect(roleFields.rules.extractor({})).toBe('');
        });
    });

    describe('rulecount', () => {
        it('counts rules correctly', () => {
            expect(roleFields.rulecount.extractor({ rules: [{}, {}, {}] })).toBe('3');
            expect(roleFields.rulecount.extractor({})).toBe('0');
        });
    });

    describe('verbs', () => {
        it('extracts unique verbs from all rules', () => {
            const role = {
                rules: [
                    { verbs: ['get', 'list'] },
                    { verbs: ['get', 'watch'] }
                ]
            };
            const result = roleFields.verbs.extractor(role);
            expect(result).toContain('get');
            expect(result).toContain('list');
            expect(result).toContain('watch');
        });

        it('handles empty rules', () => {
            expect(roleFields.verbs.extractor({ rules: [] })).toBe('');
        });
    });

    describe('resources', () => {
        it('extracts unique resources from all rules', () => {
            const role = {
                rules: [
                    { resources: ['pods', 'services'] },
                    { resources: ['pods', 'deployments'] }
                ]
            };
            const result = roleFields.resources.extractor(role);
            expect(result).toContain('pods');
            expect(result).toContain('services');
            expect(result).toContain('deployments');
        });
    });

    describe('apigroups', () => {
        it('extracts unique API groups (empty string becomes core)', () => {
            const role = {
                rules: [
                    { apiGroups: ['', 'apps'] },
                    { apiGroups: ['batch'] }
                ]
            };
            const result = roleFields.apigroups.extractor(role);
            expect(result).toContain('core');
            expect(result).toContain('apps');
            expect(result).toContain('batch');
        });
    });
});

describe('clusterRoleFields', () => {
    it('inherits roleFields', () => {
        expect(clusterRoleFields.rules).toBeDefined();
        expect(clusterRoleFields.rulecount).toBeDefined();
    });

    describe('aggregation', () => {
        it('extracts aggregation rule selectors', () => {
            const clusterRole = {
                aggregationRule: {
                    clusterRoleSelectors: [
                        { matchLabels: { 'rbac.example.com/aggregate-to-admin': 'true' } }
                    ]
                }
            };
            expect(clusterRoleFields.aggregation.extractor(clusterRole)).toContain('rbac.example.com/aggregate-to-admin=true');
        });

        it('handles missing aggregationRule', () => {
            expect(clusterRoleFields.aggregation.extractor({})).toBe('');
        });
    });
});
