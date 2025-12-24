/**
 * Role Field Mappings
 *
 * Role-specific fields for advanced search filtering.
 * Extends common fields with Role-specific extractors.
 */

import { commonFields } from './common';

export const roleFields = {
    ...commonFields,

    rules: {
        extractor: (item) => {
            const rules = item.rules || [];
            return rules.map(r => {
                const verbs = (r.verbs || []).join(',');
                const resources = (r.resources || []).join(',');
                const apiGroups = (r.apiGroups || ['']).join(',');
                return `${apiGroups}/${resources}:${verbs}`;
            }).join(' ');
        },
        aliases: ['rule']
    },

    rulecount: {
        extractor: (item) => String((item.rules || []).length),
        aliases: ['rulescount', 'numrules']
    },

    verbs: {
        extractor: (item) => {
            const rules = item.rules || [];
            const allVerbs = new Set();
            rules.forEach(r => (r.verbs || []).forEach(v => allVerbs.add(v)));
            return Array.from(allVerbs).join(' ');
        },
        aliases: ['verb']
    },

    resources: {
        extractor: (item) => {
            const rules = item.rules || [];
            const allResources = new Set();
            rules.forEach(r => (r.resources || []).forEach(res => allResources.add(res)));
            return Array.from(allResources).join(' ');
        },
        aliases: ['resource']
    },

    apigroups: {
        extractor: (item) => {
            const rules = item.rules || [];
            const allGroups = new Set();
            rules.forEach(r => (r.apiGroups || []).forEach(g => allGroups.add(g || 'core')));
            return Array.from(allGroups).join(' ');
        },
        aliases: ['apigroup', 'groups', 'group']
    }
};
