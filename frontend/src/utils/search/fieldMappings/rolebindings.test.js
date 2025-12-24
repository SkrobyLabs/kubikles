import { describe, it, expect } from 'vitest';
import { roleBindingFields } from './rolebindings';
import { clusterRoleBindingFields } from './clusterrolebindings';

describe('roleBindingFields', () => {
    describe('common fields inheritance', () => {
        it('includes name field from common', () => {
            expect(roleBindingFields.name).toBeDefined();
            expect(roleBindingFields.name.extractor({ metadata: { name: 'my-binding' } })).toBe('my-binding');
        });

        it('includes namespace field from common', () => {
            expect(roleBindingFields.namespace).toBeDefined();
        });
    });

    describe('roleref field', () => {
        it('extracts role reference as kind/name', () => {
            const binding = {
                roleRef: { kind: 'Role', name: 'pod-reader' }
            };
            expect(roleBindingFields.roleref.extractor(binding)).toBe('Role/pod-reader');
        });

        it('handles ClusterRole reference', () => {
            const binding = {
                roleRef: { kind: 'ClusterRole', name: 'admin' }
            };
            expect(roleBindingFields.roleref.extractor(binding)).toBe('ClusterRole/admin');
        });

        it('handles missing roleRef', () => {
            expect(roleBindingFields.roleref.extractor({})).toBe('/');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.roleref.aliases).toContain('role');
            expect(roleBindingFields.roleref.aliases).toContain('ref');
        });
    });

    describe('rolerefkind field', () => {
        it('extracts role reference kind', () => {
            const binding = { roleRef: { kind: 'Role' } };
            expect(roleBindingFields.rolerefkind.extractor(binding)).toBe('Role');
        });

        it('handles missing roleRef', () => {
            expect(roleBindingFields.rolerefkind.extractor({})).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.rolerefkind.aliases).toContain('refkind');
        });
    });

    describe('rolerefname field', () => {
        it('extracts role reference name', () => {
            const binding = { roleRef: { name: 'admin' } };
            expect(roleBindingFields.rolerefname.extractor(binding)).toBe('admin');
        });

        it('handles missing roleRef', () => {
            expect(roleBindingFields.rolerefname.extractor({})).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.rolerefname.aliases).toContain('refname');
            expect(roleBindingFields.rolerefname.aliases).toContain('rolename');
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

        it('handles empty subjects', () => {
            expect(roleBindingFields.subjects.extractor({ subjects: [] })).toBe('');
        });

        it('handles missing subjects', () => {
            expect(roleBindingFields.subjects.extractor({})).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.subjects.aliases).toContain('subject');
        });
    });

    describe('subjectcount field', () => {
        it('counts subjects correctly', () => {
            const binding = { subjects: [{}, {}, {}] };
            expect(roleBindingFields.subjectcount.extractor(binding)).toBe('3');
        });

        it('returns 0 for empty subjects', () => {
            expect(roleBindingFields.subjectcount.extractor({ subjects: [] })).toBe('0');
        });

        it('returns 0 for missing subjects', () => {
            expect(roleBindingFields.subjectcount.extractor({})).toBe('0');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.subjectcount.aliases).toContain('subjectscount');
            expect(roleBindingFields.subjectcount.aliases).toContain('numsubjects');
        });
    });

    describe('subjectkind field', () => {
        it('extracts unique subject kinds', () => {
            const binding = {
                subjects: [
                    { kind: 'User', name: 'alice' },
                    { kind: 'User', name: 'bob' },
                    { kind: 'Group', name: 'admins' }
                ]
            };
            const result = roleBindingFields.subjectkind.extractor(binding);
            expect(result).toContain('User');
            expect(result).toContain('Group');
        });

        it('handles empty subjects', () => {
            expect(roleBindingFields.subjectkind.extractor({ subjects: [] })).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.subjectkind.aliases).toContain('subjectkinds');
        });
    });

    describe('subjectnames field', () => {
        it('extracts all subject names', () => {
            const binding = {
                subjects: [
                    { kind: 'User', name: 'alice' },
                    { kind: 'User', name: 'bob' }
                ]
            };
            const result = roleBindingFields.subjectnames.extractor(binding);
            expect(result).toContain('alice');
            expect(result).toContain('bob');
        });

        it('handles empty subjects', () => {
            expect(roleBindingFields.subjectnames.extractor({ subjects: [] })).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.subjectnames.aliases).toContain('subjectname');
        });
    });

    describe('users field', () => {
        it('extracts only User subjects', () => {
            const binding = {
                subjects: [
                    { kind: 'User', name: 'alice' },
                    { kind: 'Group', name: 'admins' },
                    { kind: 'User', name: 'bob' }
                ]
            };
            const result = roleBindingFields.users.extractor(binding);
            expect(result).toBe('alice bob');
            expect(result).not.toContain('admins');
        });

        it('returns empty for no User subjects', () => {
            const binding = {
                subjects: [{ kind: 'Group', name: 'admins' }]
            };
            expect(roleBindingFields.users.extractor(binding)).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.users.aliases).toContain('user');
        });
    });

    describe('groups field', () => {
        it('extracts only Group subjects', () => {
            const binding = {
                subjects: [
                    { kind: 'User', name: 'alice' },
                    { kind: 'Group', name: 'admins' },
                    { kind: 'Group', name: 'developers' }
                ]
            };
            const result = roleBindingFields.groups.extractor(binding);
            expect(result).toBe('admins developers');
            expect(result).not.toContain('alice');
        });

        it('returns empty for no Group subjects', () => {
            const binding = {
                subjects: [{ kind: 'User', name: 'alice' }]
            };
            expect(roleBindingFields.groups.extractor(binding)).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.groups.aliases).toContain('group');
        });
    });

    describe('serviceaccounts field', () => {
        it('extracts ServiceAccount subjects with namespace', () => {
            const binding = {
                subjects: [
                    { kind: 'ServiceAccount', name: 'default', namespace: 'kube-system' },
                    { kind: 'User', name: 'alice' },
                    { kind: 'ServiceAccount', name: 'my-sa', namespace: 'default' }
                ]
            };
            const result = roleBindingFields.serviceaccounts.extractor(binding);
            expect(result).toContain('kube-system/default');
            expect(result).toContain('default/my-sa');
            expect(result).not.toContain('alice');
        });

        it('handles ServiceAccount without namespace', () => {
            const binding = {
                subjects: [{ kind: 'ServiceAccount', name: 'default' }]
            };
            expect(roleBindingFields.serviceaccounts.extractor(binding)).toBe('default');
        });

        it('returns empty for no ServiceAccount subjects', () => {
            const binding = {
                subjects: [{ kind: 'User', name: 'alice' }]
            };
            expect(roleBindingFields.serviceaccounts.extractor(binding)).toBe('');
        });

        it('has correct aliases', () => {
            expect(roleBindingFields.serviceaccounts.aliases).toContain('serviceaccount');
            expect(roleBindingFields.serviceaccounts.aliases).toContain('sa');
        });
    });
});

describe('clusterRoleBindingFields', () => {
    it('inherits all roleBindingFields', () => {
        expect(clusterRoleBindingFields.roleref).toBeDefined();
        expect(clusterRoleBindingFields.subjects).toBeDefined();
        expect(clusterRoleBindingFields.subjectcount).toBeDefined();
        expect(clusterRoleBindingFields.users).toBeDefined();
        expect(clusterRoleBindingFields.groups).toBeDefined();
        expect(clusterRoleBindingFields.serviceaccounts).toBeDefined();
    });

    it('roleref extractor works correctly', () => {
        const binding = { roleRef: { kind: 'ClusterRole', name: 'cluster-admin' } };
        expect(clusterRoleBindingFields.roleref.extractor(binding)).toBe('ClusterRole/cluster-admin');
    });
});
