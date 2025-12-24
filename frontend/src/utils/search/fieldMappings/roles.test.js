import { describe, it, expect } from 'vitest';
import { roleFields } from './roles';
import { clusterRoleFields } from './clusterroles';

describe('roleFields', () => {
    describe('common fields inheritance', () => {
        it('includes name field from common', () => {
            expect(roleFields.name).toBeDefined();
            expect(roleFields.name.extractor({ metadata: { name: 'my-role' } })).toBe('my-role');
        });

        it('includes namespace field from common', () => {
            expect(roleFields.namespace).toBeDefined();
        });
    });

    describe('rules field', () => {
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

        it('handles empty apiGroups (core API)', () => {
            const role = {
                rules: [{ apiGroups: [''], resources: ['pods'], verbs: ['get'] }]
            };
            expect(roleFields.rules.extractor(role)).toBe('/pods:get');
        });

        it('handles empty rules array', () => {
            expect(roleFields.rules.extractor({ rules: [] })).toBe('');
        });

        it('handles missing rules', () => {
            expect(roleFields.rules.extractor({})).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleFields.rules.aliases).toContain('rule');
        });
    });

    describe('rulecount field', () => {
        it('counts rules correctly', () => {
            const role = { rules: [{}, {}, {}] };
            expect(roleFields.rulecount.extractor(role)).toBe('3');
        });

        it('returns 0 for empty rules', () => {
            expect(roleFields.rulecount.extractor({ rules: [] })).toBe('0');
        });

        it('returns 0 for missing rules', () => {
            expect(roleFields.rulecount.extractor({})).toBe('0');
        });

        it('has correct aliases', () => {
            expect(roleFields.rulecount.aliases).toContain('rulescount');
            expect(roleFields.rulecount.aliases).toContain('numrules');
        });
    });

    describe('verbs field', () => {
        it('extracts unique verbs from all rules', () => {
            const role = {
                rules: [
                    { verbs: ['get', 'list'] },
                    { verbs: ['get', 'watch'] },
                    { verbs: ['create'] }
                ]
            };
            const result = roleFields.verbs.extractor(role);
            expect(result).toContain('get');
            expect(result).toContain('list');
            expect(result).toContain('watch');
            expect(result).toContain('create');
        });

        it('handles empty rules', () => {
            expect(roleFields.verbs.extractor({ rules: [] })).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleFields.verbs.aliases).toContain('verb');
        });
    });

    describe('resources field', () => {
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

        it('handles empty rules', () => {
            expect(roleFields.resources.extractor({ rules: [] })).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleFields.resources.aliases).toContain('resource');
        });
    });

    describe('apigroups field', () => {
        it('extracts unique API groups from all rules', () => {
            const role = {
                rules: [
                    { apiGroups: ['', 'apps'] },
                    { apiGroups: ['batch'] }
                ]
            };
            const result = roleFields.apigroups.extractor(role);
            expect(result).toContain('core'); // empty string becomes 'core'
            expect(result).toContain('apps');
            expect(result).toContain('batch');
        });

        it('handles empty rules', () => {
            expect(roleFields.apigroups.extractor({ rules: [] })).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleFields.apigroups.aliases).toContain('apigroup');
            expect(roleFields.apigroups.aliases).toContain('groups');
            expect(roleFields.apigroups.aliases).toContain('group');
        });
    });
});

describe('clusterRoleFields', () => {
    it('inherits all roleFields', () => {
        expect(clusterRoleFields.rules).toBeDefined();
        expect(clusterRoleFields.rulecount).toBeDefined();
        expect(clusterRoleFields.verbs).toBeDefined();
        expect(clusterRoleFields.resources).toBeDefined();
        expect(clusterRoleFields.apigroups).toBeDefined();
    });

    describe('aggregation field', () => {
        it('extracts aggregation rule selectors', () => {
            const clusterRole = {
                aggregationRule: {
                    clusterRoleSelectors: [
                        { matchLabels: { 'rbac.example.com/aggregate-to-admin': 'true' } },
                        { matchLabels: { 'rbac.example.com/aggregate-to-edit': 'true' } }
                    ]
                }
            };
            const result = clusterRoleFields.aggregation.extractor(clusterRole);
            expect(result).toContain('rbac.example.com/aggregate-to-admin=true');
            expect(result).toContain('rbac.example.com/aggregate-to-edit=true');
        });

        it('handles missing aggregationRule', () => {
            expect(clusterRoleFields.aggregation.extractor({})).toBe('');
        });

        it('handles empty clusterRoleSelectors', () => {
            const clusterRole = { aggregationRule: { clusterRoleSelectors: [] } };
            expect(clusterRoleFields.aggregation.extractor(clusterRole)).toBe('');
        });

        it('has correct aliases', () => {
            expect(clusterRoleFields.aggregation.aliases).toContain('aggregate');
            expect(clusterRoleFields.aggregation.aliases).toContain('aggregationrule');
        });
    });
});
