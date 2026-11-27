import React from 'react';
import { converter, normalizeAnsiCodes, stripAnsiCodes, highlightMatchesInHtml } from './logUtils';

// Contrasting colors for pod/container name prefixes (designed for dark backgrounds)
const PREFIX_COLORS = [
    '#60a5fa', // blue-400
    '#34d399', // emerald-400
    '#f472b6', // pink-400
    '#a78bfa', // violet-400
    '#fbbf24', // amber-400
    '#2dd4bf', // teal-400
    '#fb923c', // orange-400
    '#c084fc', // purple-400
    '#4ade80', // green-400
    '#f87171', // red-400
    '#38bdf8', // sky-400
    '#e879f9', // fuchsia-400
];

// Simple hash function to get consistent color index for a name
const getColorForName = (name: any) => {
    let hash = 0;
    for (let i = 0; i < name.length; i++) {
        hash = ((hash << 5) - hash) + name.charCodeAt(i);
        hash = hash & hash; // Convert to 32bit integer
    }
    return PREFIX_COLORS[Math.abs(hash) % PREFIX_COLORS.length];
};

// Parse prefix from content: [podName/containerName] or [containerName] or [podName]
// Only match valid K8s resource names (lowercase alphanumeric, hyphens, dots) with optional /container suffix.
// This avoids matching application-level log prefixes like [INFO], [Microsoft.Hosting.Lifetime], or timestamps.
const parseLogPrefix = (content: any) => {
    const match = content.match(/^\[([a-z0-9][a-z0-9\-.]*(\/[a-z0-9][a-z0-9\-.]*)?)\]\s*/);
    if (match) {
        const prefixContent = match[1];
        const slashIndex = prefixContent.indexOf('/');

        if (slashIndex !== -1) {
            // Format: [podName/containerName]
            return {
                type: 'pod-container',
                podName: prefixContent.slice(0, slashIndex),
                containerName: prefixContent.slice(slashIndex + 1),
                fullPrefix: prefixContent,
                rest: content.slice(match[0].length)
            };
        } else {
            // Format: [name] - could be either pod or container depending on context
            return {
                type: 'single',
                name: prefixContent,
                fullPrefix: prefixContent,
                rest: content.slice(match[0].length)
            };
        }
    }
    return null;
};

// Rendering an entry is a pure function of (entry, parsePrefixes): the converter is not
// created with stream:true, so no colour state leaks between lines. Cache the result per
// entry object so re-renders (scroll, toggles, row recycling) skip the regex and ANSI work.
interface RenderedLine { prefixInfo: ReturnType<typeof parseLogPrefix>; normalized: string; html: string; }
const renderedPlain = new WeakMap<object, RenderedLine>();
const renderedPrefixed = new WeakMap<object, RenderedLine>();

const getRenderedLine = (entry: any, parsePrefixes: boolean): RenderedLine => {
    const cache = parsePrefixes ? renderedPrefixed : renderedPlain;
    let rendered = cache.get(entry);
    if (!rendered) {
        // Parse prefix only when viewing multiple pods or containers
        const prefixInfo = parsePrefixes ? parseLogPrefix(entry.content) : null;
        // Get content without prefix for ANSI processing
        const normalized = normalizeAnsiCodes(prefixInfo ? prefixInfo.rest : entry.content);
        rendered = { prefixInfo, normalized, html: converter.toHtml(normalized) };
        cache.set(entry, rendered);
    }
    return rendered;
};

function SkipIndicator({ count }: { count: number }) {
    return (
        <div className="flex items-center justify-center py-1 text-gray-500 text-xs italic select-none">
            <span className="px-2 bg-gray-800/50 rounded">
                ... skipped {count} line{count !== 1 ? 's' : ''} ...
            </span>
        </div>
    );
}

interface LogLineProps {
    entry: any;
    showTimestamps: any;
    searchTerm: any;
    searchRegex: any;
    wrapLines: any;
    parsePrefixes?: boolean;
    /** Stable line id, rendered as data-log-idx. Falls back to entry.originalIndex. */
    lineId?: number;
    /** Whether the line matches the search. Falls back to entry.isMatch. */
    isMatch?: boolean;
}

// Search props only matter to matching lines, so a term change must not re-render the rest.
const logLinePropsEqual = (prev: LogLineProps, next: LogLineProps) => {
    if (prev.entry !== next.entry
        || prev.showTimestamps !== next.showTimestamps
        || prev.wrapLines !== next.wrapLines
        || prev.parsePrefixes !== next.parsePrefixes
        || prev.lineId !== next.lineId
        || prev.isMatch !== next.isMatch) return false;
    const matched = next.isMatch ?? next.entry.isMatch;
    if (matched && (prev.searchTerm !== next.searchTerm || prev.searchRegex !== next.searchRegex)) return false;
    return true;
};

/**
 * Renders a single log line with ANSI color support and search highlighting.
 * Memoized so appends and unrelated prop changes do not re-render existing lines.
 */
export const LogLine = React.memo(function LogLine({
    entry,
    showTimestamps,
    searchTerm,
    searchRegex,
    wrapLines,
    parsePrefixes,
    lineId,
    isMatch
}: LogLineProps) {
    // Skip indicator (for filtered view)
    if (entry.isSkipIndicator) return <SkipIndicator count={entry.skippedCount} />;

    const matched = isMatch ?? entry.isMatch;
    const { prefixInfo, normalized, html } = getRenderedLine(entry, !!parsePrefixes);
    let htmlContent = html;

    // If searching and this line matches, highlight the matches while preserving ANSI colors
    if (searchTerm && searchRegex && matched) {
        const strippedContent = stripAnsiCodes(normalized);
        htmlContent = highlightMatchesInHtml(html, strippedContent, searchRegex);
    }

    // Render the prefix with appropriate coloring
    const renderPrefix = () => {
        if (!prefixInfo) return null;

        if (prefixInfo.type === 'pod-container') {
            // Format: [podName/containerName] - color each part differently
            return (
                <span className="select-none mr-1 shrink-0 font-medium">
                    [<span style={{ color: getColorForName(prefixInfo.podName) }}>{prefixInfo.podName}</span>
                    <span className="text-gray-500">/</span>
                    <span style={{ color: getColorForName(prefixInfo.containerName) }}>{prefixInfo.containerName}</span>]
                </span>
            );
        } else {
            // Format: [name] - single color
            return (
                <span
                    className="select-none mr-1 shrink-0 font-medium"
                    style={{ color: getColorForName(prefixInfo.name) }}
                >
                    [{prefixInfo.name}]
                </span>
            );
        }
    };

    return (
        <div
            data-log-idx={lineId ?? entry.originalIndex}
            className={`flex ${matched ? 'bg-yellow-500/10' : ''} ${wrapLines ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'}`}
        >
            {showTimestamps && entry.timestamp && (
                <span className="text-gray-500 select-none mr-2 shrink-0">
                    {entry.timestamp}
                </span>
            )}
            {renderPrefix()}
            <span data-log-content="" dangerouslySetInnerHTML={{ __html: htmlContent }} />
        </div>
    );
}, logLinePropsEqual);

/**
 * Spinner component for loading states.
 */
export function Spinner({ className = "w-4 h-4" }: any) {
    return (
        <svg className={`${className} animate-spin`} fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
    );
}
