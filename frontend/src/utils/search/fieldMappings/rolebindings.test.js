import { describe, it, expect } from 'vitest';
import { roleBindingFields } from './rolebindings';
import { clusterRoleBindingFields } from './clusterrolebindings';

describe('roleBindingFields', () => {
    describe('roleref fields', () => {
        const binding = { roleRef: { kind: 'Role', name: 'pod-reader' } };

        it('extracts role reference', () => {
            expect(roleBindingFields.roleref.extractor(binding)).toBe('Role/pod-reader');
            expect(roleBindingFields.rolerefkind.extractor(binding)).toBe('Role');
            expect(roleBindingFields.rolerefname.extractor(binding)).toBe('pod-reader');
        });

        it('handles missing roleRef', () => {
            expect(roleBindingFields.roleref.extractor({})).toBe('/');
            expect(roleBindingFields.rolerefkind.extractor({})).toBe('');
            expect(roleBindingFields.rolerefname.extractor({})).toBe('');
        });
    });

    describe('subjects field', () => {
        it('extracts all subjects as kind/name', () => {
            const binding = {
                subjects: [
                    { kind: 'User', name: 'alice' },
                    { kind: 'ServiceAccount', name: 'default' }
                ]
            };
            const result = roleBindingFields.subjects.extractor(binding);
            expect(result).toContain('User/alice');
            expect(result).toContain('ServiceAccount/default');
        });

        it('handles empty or missing subjects', () => {
            expect(roleBindingFields.subjects.extractor({ subjects: [] })).toBe('');
            expect(roleBindingFields.subjects.extractor({})).toBe('');
        });
    });

    describe('subjectcount', () => {
        it('counts subjects correctly', () => {
            expect(roleBindingFields.subjectcount.extractor({ subjects: [{}, {}, {}] })).toBe('3');
            expect(roleBindingFields.subjectcount.extractor({})).toBe('0');
        });
    });

    describe('subject filtering fields', () => {
        const binding = {
            subjects: [
                { kind: 'User', name: 'alice' },
                { kind: 'User', name: 'bob' },
                { kind: 'Group', name: 'admins' },
                { kind: 'ServiceAccount', name: 'default', namespace: 'kube-system' }
            ]
        };

        it('extracts unique subject kinds', () => {
            const result = roleBindingFields.subjectkind.extractor(binding);
            expect(result).toContain('User');
            expect(result).toContain('Group');
            expect(result).toContain('ServiceAccount');
        });

        it('extracts all subject names', () => {
            const result = roleBindingFields.subjectnames.extractor(binding);
            expect(result).toContain('alice');
            expect(result).toContain('bob');
            expect(result).toContain('admins');
        });

        it('filters users only', () => {
            expect(roleBindingFields.users.extractor(binding)).toBe('alice bob');
        });

        it('filters groups only', () => {
            expect(roleBindingFields.groups.extractor(binding)).toBe('admins');
        });

        it('filters serviceaccounts with namespace', () => {
            expect(roleBindingFields.serviceaccounts.extractor(binding)).toBe('kube-system/default');
        });

        it('handles empty subjects for all filters', () => {
            expect(roleBindingFields.subjectkind.extractor({})).toBe('');
            expect(roleBindingFields.subjectnames.extractor({})).toBe('');
            expect(roleBindingFields.users.extractor({})).toBe('');
            expect(roleBindingFields.groups.extractor({})).toBe('');
            expect(roleBindingFields.serviceaccounts.extractor({})).toBe('');
        });
    });
});

describe('clusterRoleBindingFields', () => {
    it('inherits roleBindingFields and works correctly', () => {
        expect(clusterRoleBindingFields.roleref).toBeDefined();
        expect(clusterRoleBindingFields.subjects).toBeDefined();

        const binding = { roleRef: { kind: 'ClusterRole', name: 'cluster-admin' } };
        expect(clusterRoleBindingFields.roleref.extractor(binding)).toBe('ClusterRole/cluster-admin');
    });
});
