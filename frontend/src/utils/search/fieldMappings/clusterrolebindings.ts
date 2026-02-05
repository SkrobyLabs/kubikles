/**
 * ClusterRoleBinding Field Mappings
 *
 * ClusterRoleBinding-specific fields for advanced search filtering.
 * Uses same fields as RoleBinding since structure is identical.
 */

import { roleBindingFields } from './rolebindings';

// ClusterRoleBindings have the same structure as RoleBindings
export const clusterRoleBindingFields = {
    ...roleBindingFields
};
