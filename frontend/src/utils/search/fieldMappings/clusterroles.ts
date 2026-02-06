/**
 * ClusterRole Field Mappings
 *
 * ClusterRole-specific fields for advanced search filtering.
 * Uses same fields as Role since structure is identical.
 */

import { roleFields } from './roles';

// ClusterRoles have the same structure as Roles
export const clusterRoleFields = {
    ...roleFields,

    aggregation: {
        extractor: (item: any) => {
            const rules = item.aggregationRule?.clusterRoleSelectors || [];
            return rules.map((r: any) => {
                const matchLabels = r.matchLabels || {};
                return Object.entries(matchLabels).map(([k, v]) => `${k}=${v}`).join(',');
            }).join(' ');
        },
        aliases: ['aggregate', 'aggregationrule']
    }
};
