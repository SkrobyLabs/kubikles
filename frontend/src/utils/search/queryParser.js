/**
 * Query Parser for Advanced Search
 *
 * Parses search query strings into structured conditions.
 *
 * Supported syntax:
 * - Plain text: "my-pod" -> matches name containing "my-pod"
 * - Field-specific: name:"my-pod", nodeName:"node1"
 * - Regex: name:/^prefix/, status:/running/i
 * - Multiple conditions: name:"api" status:"Running" (AND-ed)
 * - OR groups: name:/^web-/ OR name:/^api-/ (OR-ed groups)
 */

/**
 * Parse a search query string into groups of conditions.
 * Groups are OR-ed together, conditions within a group are AND-ed.
 *
 * @param {string} queryString - The search query string
 * @returns {{groups: Array<Array<{type: string, field?: string, value: string|RegExp, isRegex: boolean}>>}}
 *
 * @example
 * parseQuery('name:"nginx" status:Running')
 * // Returns: { groups: [[cond1, cond2]] }  // Single group, AND-ed
 *
 * parseQuery('name:/^web-/ OR name:/^api-/')
 * // Returns: { groups: [[cond1], [cond2]] }  // Two groups, OR-ed
 */
export function parseQuery(queryString) {
    if (!queryString || typeof queryString !== 'string') {
        return { groups: [] };
    }

    const query = queryString.trim();
    if (!query) return { groups: [] };

    // Split by OR (case-insensitive, surrounded by whitespace)
    const orSegments = query.split(/\s+OR\s+/i);

    const groups = orSegments
        .map(segment => parseSegment(segment.trim()))
        .filter(conditions => conditions.length > 0);

    return { groups };
}

/**
 * Parse a single segment (no OR) into an array of AND-ed conditions.
 *
 * @param {string} segment - Query segment without OR
 * @returns {Array<{type: string, field?: string, value: string|RegExp, isRegex: boolean}>}
 */
function parseSegment(segment) {
    if (!segment) return [];

    const conditions = [];
    let remaining = segment;

    while (remaining.length > 0) {
        remaining = remaining.trimStart();
        if (!remaining) break;

        // Try to match field:value patterns
        const fieldMatch = matchFieldCondition(remaining);

        if (fieldMatch) {
            conditions.push(fieldMatch.condition);
            remaining = remaining.slice(fieldMatch.consumed);
        } else {
            // Plain text - extract next word (stop at whitespace or field pattern)
            const wordMatch = remaining.match(/^([^\s:]+)(?=\s|$|[a-zA-Z]+:)/);
            if (wordMatch) {
                conditions.push({
                    type: 'plain',
                    value: wordMatch[1],
                    isRegex: false
                });
                remaining = remaining.slice(wordMatch[1].length);
            } else {
                // Fallback: take everything up to next whitespace
                const fallbackMatch = remaining.match(/^(\S+)/);
                if (fallbackMatch) {
                    conditions.push({
                        type: 'plain',
                        value: fallbackMatch[1],
                        isRegex: false
                    });
                    remaining = remaining.slice(fallbackMatch[1].length);
                } else {
                    break;
                }
            }
        }
    }

    return conditions;
}

/**
 * Try to match a field:value condition at the start of the string.
 *
 * @param {string} str - String to parse
 * @returns {{condition: object, consumed: number}|null}
 */
function matchFieldCondition(str) {
    // Match field name followed by colon
    const fieldNameMatch = str.match(/^(\w+):/);
    if (!fieldNameMatch) return null;

    const fieldName = fieldNameMatch[1].toLowerCase();
    let pos = fieldNameMatch[0].length;
    const afterColon = str.slice(pos);

    // Try regex pattern: /pattern/flags
    const regexMatch = afterColon.match(/^\/(.+?)\/([gimsuy]*)?(?=\s|,|$)/);
    if (regexMatch) {
        const pattern = regexMatch[1];
        const flags = regexMatch[2] || '';
        try {
            const regex = new RegExp(pattern, flags);
            return {
                condition: {
                    type: 'field',
                    field: fieldName,
                    value: regex,
                    isRegex: true
                },
                consumed: pos + regexMatch[0].length
            };
        } catch (e) {
            // Invalid regex - treat as literal string
            return {
                condition: {
                    type: 'field',
                    field: fieldName,
                    value: regexMatch[0],
                    isRegex: false
                },
                consumed: pos + regexMatch[0].length
            };
        }
    }

    // Try double-quoted value: "value"
    const doubleQuoteMatch = afterColon.match(/^"([^"]*)"/);
    if (doubleQuoteMatch) {
        return {
            condition: {
                type: 'field',
                field: fieldName,
                value: doubleQuoteMatch[1],
                isRegex: false
            },
            consumed: pos + doubleQuoteMatch[0].length
        };
    }

    // Try single-quoted value: 'value'
    const singleQuoteMatch = afterColon.match(/^'([^']*)'/);
    if (singleQuoteMatch) {
        return {
            condition: {
                type: 'field',
                field: fieldName,
                value: singleQuoteMatch[1],
                isRegex: false
            },
            consumed: pos + singleQuoteMatch[0].length
        };
    }

    // Unquoted value: take until whitespace or comma
    const unquotedMatch = afterColon.match(/^([^\s,]+)/);
    if (unquotedMatch) {
        return {
            condition: {
                type: 'field',
                field: fieldName,
                value: unquotedMatch[1],
                isRegex: false
            },
            consumed: pos + unquotedMatch[0].length
        };
    }

    return null;
}
