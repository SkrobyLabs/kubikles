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

    // Field definitions and lowercase needles are resolved once here, not per item.
    const groups = parsed.groups.map((group: any) =>
        group.map((condition: any) => compileCondition(condition, resourceType)));

    return (item: any) => {
        // Groups are OR-ed: if ANY group matches, the item passes
        // Conditions within a group are AND-ed: ALL must match
        for (const group of groups) {
            let all = true;
            for (const test of group) {
                if (!test(item)) { all = false; break; }
            }
            if (all) return true;
        }
        return false;
    };
}

/**
 * Compile a single parsed condition into a predicate.
 *
 * @param {object} condition - The parsed condition
 * @param {string} resourceType - The resource type
 */
function compileCondition(condition: any, resourceType: string): (item: any) => boolean {
    if (condition.type === 'plain') {
        // Plain text matches name only (backward compatible)
        if (condition.isRegex) {
            return (item: any) => testRegex(condition.value, item.metadata?.name || '');
        }

        const needle = condition.value.toLowerCase();
        return (item: any) => (item.metadata?.name || '').toLowerCase().includes(needle);
    }

    if (condition.type === 'field') {
        const fieldDef = getFieldByName(resourceType, condition.field);

        if (!fieldDef) {
            // Unknown field - no match
            return () => false;
        }

        const extract = fieldDef.extractor;

        if (condition.isRegex) {
            return (item: any) => testRegex(condition.value, extract(item));
        }

        // Case-insensitive partial match
        const needle = condition.value.toLowerCase();
        return (item: any) => extract(item).toLowerCase().includes(needle);
    }

    return () => false;
}

function testRegex(regex: any, value: any): boolean {
    if (!(regex instanceof RegExp)) return false;

    try {
        regex.lastIndex = 0;
        return regex.test(String(value || ''));
    } catch (e: any) {
        return false;
    }
}
