/**
 * Field Mappings Registry
 *
 * Central registry for resource-specific field mappings.
 * Handles field lookup by name or alias.
 */

import { podFields } from './pods';
import { deploymentFields } from './deployments';
import { statefulSetFields } from './statefulsets';
import { daemonSetFields } from './daemonsets';
import { replicaSetFields } from './replicasets';
import { jobFields } from './jobs';
import { cronJobFields } from './cronjobs';
import { nodeFields } from './nodes';
import { serviceFields } from './services';
import { secretFields } from './secrets';
import { configMapFields } from './configmaps';
import { namespaceFields } from './namespaces';
import { eventFields } from './events';
import { commonFields } from './common';

const registry = {
    pods: podFields,
    deployments: deploymentFields,
    statefulsets: statefulSetFields,
    daemonsets: daemonSetFields,
    replicasets: replicaSetFields,
    jobs: jobFields,
    cronjobs: cronJobFields,
    nodes: nodeFields,
    services: serviceFields,
    secrets: secretFields,
    configmaps: configMapFields,
    namespaces: namespaceFields,
    events: eventFields,
};

/**
 * Get all field mappings for a resource type.
 *
 * @param {string} resourceType - The resource type (e.g., 'pods')
 * @returns {object} Field mappings object
 */
export function getFieldsForResource(resourceType) {
    return registry[resourceType] || commonFields;
}

/**
 * Get a specific field definition by name or alias.
 *
 * @param {string} resourceType - The resource type (e.g., 'pods')
 * @param {string} fieldName - The field name or alias to look up
 * @returns {object|null} Field definition or null if not found
 */
export function getFieldByName(resourceType, fieldName) {
    const fields = getFieldsForResource(resourceType);
    const normalizedName = fieldName.toLowerCase();

    // Direct match
    if (fields[normalizedName]) {
        return fields[normalizedName];
    }

    // Alias match
    for (const [key, field] of Object.entries(fields)) {
        if (field.aliases && field.aliases.includes(normalizedName)) {
            return field;
        }
    }

    return null;
}

/**
 * Get list of all available field names for a resource type.
 * Includes primary names and aliases.
 *
 * @param {string} resourceType - The resource type
 * @returns {string[]} Array of field names and aliases
 */
export function getAvailableFieldNames(resourceType) {
    const fields = getFieldsForResource(resourceType);
    const names = [];

    for (const [key, field] of Object.entries(fields)) {
        names.push(key);
        if (field.aliases) {
            names.push(...field.aliases);
        }
    }

    return names;
}

/**
 * Get field definitions with metadata for autocomplete.
 *
 * @param {string} resourceType - The resource type
 * @returns {Array<{name: string, aliases: string[]}>}
 */
export function getFieldsMetadata(resourceType) {
    const fields = getFieldsForResource(resourceType);

    return Object.entries(fields).map(([name, field]) => ({
        name,
        aliases: field.aliases || []
    }));
}
