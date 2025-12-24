import Convert from 'ansi-to-html';

/**
 * ANSI to HTML converter instance with dark theme settings.
 */
export const converter = new Convert({
    fg: '#FFF',
    bg: '#1e1e1e',
    newline: true,
    escapeXML: true
});

/**
 * Fix non-standard 4-digit 256-color codes (e.g., 0008 -> 8) to standard format.
 * Some log sources emit non-standard ANSI codes that need normalization.
 */
export const normalizeAnsiCodes = (text) => {
    return text.replace(/\x1b\[38;5;0*(\d{1,3})m/g, '\x1b[38;5;$1m')
               .replace(/\x1b\[48;5;0*(\d{1,3})m/g, '\x1b[48;5;$1m');
};

/**
 * Strip all ANSI escape codes from text (for search matching).
 */
export const stripAnsiCodes = (text) => {
    // eslint-disable-next-line no-control-regex
    return text.replace(/\x1b\[[0-9;]*m/g, '');
};

/**
 * Validate RFC3339 datetime format (e.g., 2024-11-26T14:30:00Z).
 * Accepts multiple common formats.
 */
export const isValidDateTime = (str) => {
    if (!str) return false;
    // Accept formats like: 2024-11-26T14:30:00Z, 2024-11-26T14:30:00, 2024-11-26 14:30:00
    const patterns = [
        /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z?$/,
        /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/,
        /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/,
        /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/
    ];
    if (!patterns.some(p => p.test(str))) return false;
    const date = new Date(str.replace(' ', 'T'));
    return !isNaN(date.getTime());
};

/**
 * Convert input datetime string to RFC3339 format.
 */
export const toRFC3339 = (str) => {
    if (!str) return '';
    let normalized = str.replace(' ', 'T');
    if (!normalized.includes(':00Z') && !normalized.endsWith('Z')) {
        if (normalized.length === 16) { // 2024-11-26T14:30
            normalized += ':00Z';
        } else if (normalized.length === 19) { // 2024-11-26T14:30:00
            normalized += 'Z';
        }
    }
    return normalized;
};

/**
 * Log entry structure: { timestamp: string, content: string, source: 'initial'|'before'|'after'|'stream' }
 */

/**
 * Parse a raw log string (with timestamps) into structured log entries.
 * @param {string} rawLogs - Raw log string with newlines
 * @param {string} source - Source identifier for the log entries
 * @returns {Array<{timestamp: string, content: string, source: string}>}
 */
export const parseLogLines = (rawLogs, source) => {
    if (!rawLogs) return [];
    return rawLogs.split('\n')
        .filter(line => line.trim())
        .map(line => {
            // Match K8s timestamp format: 2024-11-26T14:30:00.123456789Z
            const match = line.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s*(.*)/);
            if (match) {
                return { timestamp: match[1], content: match[2], source };
            }
            // Line without timestamp (shouldn't happen if we always fetch with timestamps)
            return { timestamp: '', content: line, source };
        });
};

/**
 * Highlight search matches in HTML while preserving ANSI color tags.
 * This handles cases where ANSI codes split text (e.g., ERR<code>]<code>text can match ERR]text).
 * @param {string} html - HTML string with ANSI-converted color spans
 * @param {string} plainText - Plain text without ANSI codes (for matching positions)
 * @param {RegExp} searchRegex - Search regex pattern
 * @returns {string} HTML with highlighted matches
 */
export const highlightMatchesInHtml = (html, plainText, searchRegex) => {
    if (!searchRegex) return html;

    // Find all matches in plain text (ANSI-stripped)
    searchRegex.lastIndex = 0;
    const matches = [];
    let match;
    while ((match = searchRegex.exec(plainText)) !== null) {
        matches.push({ start: match.index, end: match.index + match[0].length });
        if (match[0].length === 0) break;
    }

    if (matches.length === 0) return html;

    // Walk through HTML and plain text in parallel, inserting highlight marks
    // Key insight: HTML tags don't correspond to any plain text characters,
    // so we copy them while maintaining the highlight state (close before, reopen after)
    let result = '';
    let plainIdx = 0;
    let htmlIdx = 0;
    let matchIdx = 0;
    let inHighlight = false;

    while (htmlIdx < html.length && matchIdx <= matches.length) {
        // Check if we're at an HTML tag
        if (html[htmlIdx] === '<') {
            const tagEnd = html.indexOf('>', htmlIdx);
            if (tagEnd !== -1) {
                const tag = html.slice(htmlIdx, tagEnd + 1);
                // If we're in a highlight and this is a closing/opening span tag,
                // we need to close the mark, emit the tag, then reopen the mark
                if (inHighlight) {
                    result += '</mark>';
                    result += tag;
                    result += '<mark class="bg-yellow-500/50 text-inherit">';
                } else {
                    result += tag;
                }
                htmlIdx = tagEnd + 1;
                continue;
            }
        }

        // Check if we're at an HTML entity (e.g., &lt;, &gt;, &amp;)
        if (html[htmlIdx] === '&') {
            const entityEnd = html.indexOf(';', htmlIdx);
            if (entityEnd !== -1 && entityEnd - htmlIdx < 10) {
                // Check if we need to start highlight
                if (!inHighlight && matchIdx < matches.length && plainIdx === matches[matchIdx].start) {
                    result += '<mark class="bg-yellow-500/50 text-inherit">';
                    inHighlight = true;
                }

                // Copy the entity
                result += html.slice(htmlIdx, entityEnd + 1);
                htmlIdx = entityEnd + 1;
                plainIdx++; // Entity represents one character in plain text

                // Check if we need to end highlight
                if (inHighlight && matchIdx < matches.length && plainIdx === matches[matchIdx].end) {
                    result += '</mark>';
                    inHighlight = false;
                    matchIdx++;
                }
                continue;
            }
        }

        // Regular character
        // Check if we need to start highlight
        if (!inHighlight && matchIdx < matches.length && plainIdx === matches[matchIdx].start) {
            result += '<mark class="bg-yellow-500/50 text-inherit">';
            inHighlight = true;
        }

        result += html[htmlIdx];
        htmlIdx++;
        plainIdx++;

        // Check if we need to end highlight
        if (inHighlight && matchIdx < matches.length && plainIdx === matches[matchIdx].end) {
            result += '</mark>';
            inHighlight = false;
            matchIdx++;
        }
    }

    // Copy any remaining HTML
    if (htmlIdx < html.length) {
        result += html.slice(htmlIdx);
    }

    // Close any unclosed highlight
    if (inHighlight) {
        result += '</mark>';
    }

    return result;
};

/**
 * Convert structured logs to visible format (what user sees - respects showTimestamps setting).
 */
export const logsToVisibleString = (logs, showTimestamps) => {
    return logs.map(entry => {
        if (showTimestamps && entry.timestamp) {
            return `${entry.timestamp} ${entry.content}`;
        }
        return entry.content;
    }).join('\n');
};

/**
 * Convert structured logs to debug format (includes source markers).
 */
export const logsToDebugString = (logs) => {
    return logs.map(entry => {
        const sourceMarker = `[${entry.source.toUpperCase()}]`;
        if (entry.timestamp) {
            return `${entry.timestamp} ${sourceMarker} ${entry.content}`;
        }
        return `${sourceMarker} ${entry.content}`;
    }).join('\n');
};
