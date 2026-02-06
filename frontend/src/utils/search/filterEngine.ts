/**
 * Filter Engine
 *
 * Creates filter functions from parsed query conditions.
 * Applies conditions to filter resource data.
 */

import { parseQuery } from './queryParser';
import { getFieldByName } from './fieldMappings';

/**
 * Create a filter function for the given resource type and query string.
 * Supports OR groups: groups are OR-ed together, conditions within a group are AND-ed.
 *
 * @param {string} resourceType - The resource type (e.g., 'pods')
 * @param {string} queryString - The search query string
 * @returns {function(item): boolean} Filter function
 *
 * @example
 * // Simple AND query
 * createFilter('pods', 'name:"nginx" status:Running')
 *
 * // OR query - match either pattern
 * createFilter('pods', 'name:/^web-/ OR name:/^api-/')
 */
export function createFilter(resourceType: string, queryString: string): (item: any) => boolean {
    const parsed = parseQuery(queryString);

    // No groups = match everything
    if (!parsed.groups || parsed.groups.length === 0) {
        return () => true;
    }

    return (item: any) => {
        // Groups are OR-ed: if ANY group matches, the item passes
        return parsed.groups.some((group: any) => {
            // Conditions within a group are AND-ed: ALL must match
            return group.every((condition: any) => matchCondition(item, condition, resourceType));
        });
    };
}

/**
 * Check if an item matches a single condition.
 *
 * @param {object} item - The resource item to check
 * @param {object} condition - The parsed condition
 * @param {string} resourceType - The resource type
 * @returns {boolean}
 */
function matchCondition(item: any, condition: any, resourceType: string): boolean {
    if (condition.type === 'plain') {
        // Plain text matches name only (backward compatible)
        const name = item.metadata?.name || '';
        return name.toLowerCase().includes(condition.value.toLowerCase());
    }

    if (condition.type === 'field') {
        const fieldDef = getFieldByName(resourceType, condition.field);

        if (!fieldDef) {
            // Unknown field - no match
            return false;
        }

        const fieldValue = fieldDef.extractor(item);

        if (condition.isRegex) {
            // Regex match
            try {
                return condition.value.test(fieldValue);
            } catch (e: any) {
                return false;
            }
        } else {
            // Case-insensitive partial match
            return fieldValue.toLowerCase().includes(condition.value.toLowerCase());
        }
    }

    return false;
}

/**
 * Parse and filter data in one step (convenience function).
 *
 * @param {Array} data - Array of resource items to filter
 * @param {string} resourceType - The resource type
 * @param {string} queryString - The search query string
 * @returns {Array} Filtered array
 */
export function filterData(data: any[], resourceType: string, queryString: string): any[] {
    const filterFn = createFilter(resourceType, queryString);
    return data.filter(filterFn);
}
