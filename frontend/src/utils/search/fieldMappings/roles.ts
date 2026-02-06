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
        extractor: (item: any) => {
            const rules = item.rules || [];
            return rules.map((r: any) => {
                const verbs = (r.verbs || []).join(',');
                const resources = (r.resources || []).join(',');
                const apiGroups = (r.apiGroups || ['']).join(',');
                return `${apiGroups}/${resources}:${verbs}`;
            }).join(' ');
        },
        aliases: ['rule']
    },

    rulecount: {
        extractor: (item: any) => String((item.rules || []).length),
        aliases: ['rulescount', 'numrules']
    },

    verbs: {
        extractor: (item: any) => {
            const rules = item.rules || [];
            const allVerbs = new Set<any>();
            rules.forEach((r: any) => (r.verbs || []).forEach((v: any) => allVerbs.add(v)));
            return Array.from(allVerbs).join(' ');
        },
        aliases: ['verb']
    },

    resources: {
        extractor: (item: any) => {
            const rules = item.rules || [];
            const allResources = new Set<any>();
            rules.forEach((r: any) => (r.resources || []).forEach((res: any) => allResources.add(res)));
            return Array.from(allResources).join(' ');
        },
        aliases: ['resource']
    },

    apigroups: {
        extractor: (item: any) => {
            const rules = item.rules || [];
            const allGroups = new Set<any>();
            rules.forEach((r: any) => (r.apiGroups || []).forEach((g: any) => allGroups.add(g || 'core')));
            return Array.from(allGroups).join(' ');
        },
        aliases: ['apigroup', 'groups', 'group']
    }
};
