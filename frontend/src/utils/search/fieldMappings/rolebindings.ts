/**
 * RoleBinding Field Mappings
 *
 * RoleBinding-specific fields for advanced search filtering.
 * Extends common fields with RoleBinding-specific extractors.
 */

import { commonFields } from './common';

export const roleBindingFields = {
    ...commonFields,

    roleref: {
        extractor: (item: any) => {
            const ref = item.roleRef || {};
            return `${ref.kind || ''}/${ref.name || ''}`;
        },
        aliases: ['role', 'ref']
    },

    rolerefkind: {
        extractor: (item: any) => item.roleRef?.kind || '',
        aliases: ['refkind']
    },

    rolerefname: {
        extractor: (item: any) => item.roleRef?.name || '',
        aliases: ['refname', 'rolename']
    },

    subjects: {
        extractor: (item: any) => {
            const subjects = item.subjects || [];
            return subjects.map((s: any) => `${s.kind}/${s.name}`).join(' ');
        },
        aliases: ['subject']
    },

    subjectcount: {
        extractor: (item: any) => String((item.subjects || []).length),
        aliases: ['subjectscount', 'numsubjects']
    },

    subjectkind: {
        extractor: (item: any) => {
            const subjects = item.subjects || [];
            const kinds = new Set(subjects.map((s: any) => s.kind));
            return Array.from(kinds).join(' ');
        },
        aliases: ['subjectkinds']
    },

    subjectnames: {
        extractor: (item: any) => {
            const subjects = item.subjects || [];
            return subjects.map((s: any) => s.name).join(' ');
        },
        aliases: ['subjectname']
    },

    users: {
        extractor: (item: any) => {
            const subjects = item.subjects || [];
            return subjects.filter((s: any) => s.kind === 'User').map((s: any) => s.name).join(' ');
        },
        aliases: ['user']
    },

    groups: {
        extractor: (item: any) => {
            const subjects = item.subjects || [];
            return subjects.filter((s: any) => s.kind === 'Group').map((s: any) => s.name).join(' ');
        },
        aliases: ['group']
    },

    serviceaccounts: {
        extractor: (item: any) => {
            const subjects = item.subjects || [];
            return subjects.filter((s: any) => s.kind === 'ServiceAccount')
                .map((s: any) => s.namespace ? `${s.namespace}/${s.name}` : s.name).join(' ');
        },
        aliases: ['serviceaccount', 'sa']
    }
};
