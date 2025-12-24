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
        extractor: (item) => {
            const ref = item.roleRef || {};
            return `${ref.kind || ''}/${ref.name || ''}`;
        },
        aliases: ['role', 'ref']
    },

    rolerefkind: {
        extractor: (item) => item.roleRef?.kind || '',
        aliases: ['refkind']
    },

    rolerefname: {
        extractor: (item) => item.roleRef?.name || '',
        aliases: ['refname', 'rolename']
    },

    subjects: {
        extractor: (item) => {
            const subjects = item.subjects || [];
            return subjects.map(s => `${s.kind}/${s.name}`).join(' ');
        },
        aliases: ['subject']
    },

    subjectcount: {
        extractor: (item) => String((item.subjects || []).length),
        aliases: ['subjectscount', 'numsubjects']
    },

    subjectkind: {
        extractor: (item) => {
            const subjects = item.subjects || [];
            const kinds = new Set(subjects.map(s => s.kind));
            return Array.from(kinds).join(' ');
        },
        aliases: ['subjectkinds']
    },

    subjectnames: {
        extractor: (item) => {
            const subjects = item.subjects || [];
            return subjects.map(s => s.name).join(' ');
        },
        aliases: ['subjectname']
    },

    users: {
        extractor: (item) => {
            const subjects = item.subjects || [];
            return subjects.filter(s => s.kind === 'User').map(s => s.name).join(' ');
        },
        aliases: ['user']
    },

    groups: {
        extractor: (item) => {
            const subjects = item.subjects || [];
            return subjects.filter(s => s.kind === 'Group').map(s => s.name).join(' ');
        },
        aliases: ['group']
    },

    serviceaccounts: {
        extractor: (item) => {
            const subjects = item.subjects || [];
            return subjects.filter(s => s.kind === 'ServiceAccount')
                .map(s => s.namespace ? `${s.namespace}/${s.name}` : s.name).join(' ');
        },
        aliases: ['serviceaccount', 'sa']
    }
};
