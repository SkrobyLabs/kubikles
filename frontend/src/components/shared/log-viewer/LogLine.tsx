import React, { useMemo } from 'react';
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
const getColorForName = (name) => {
    let hash = 0;
    for (let i = 0; i < name.length; i++) {
        hash = ((hash << 5) - hash) + name.charCodeAt(i);
        hash = hash & hash; // Convert to 32bit integer
    }
    return PREFIX_COLORS[Math.abs(hash) % PREFIX_COLORS.length];
};

// Parse prefix from content: [podName/containerName] or [containerName] or [podName]
const parseLogPrefix = (content) => {
    const match = content.match(/^\[([^\]]+)\]\s*/);
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

/**
 * Renders a single log line with ANSI color support and search highlighting.
 * Memoized to prevent re-renders when parent re-renders with same props.
 */
export const LogLine = React.memo(function LogLine({
    entry,
    showTimestamps,
    searchTerm,
    searchRegex,
    wrapLines
}) {
    // Skip indicator (for filtered view)
    if (entry.isSkipIndicator) {
        return (
            <div className="flex items-center justify-center py-1 text-gray-500 text-xs italic select-none">
                <span className="px-2 bg-gray-800/50 rounded">
                    ... skipped {entry.skippedCount} line{entry.skippedCount !== 1 ? 's' : ''} ...
                </span>
            </div>
        );
    }

    // Parse prefix if present (for "All Containers" or "All Pods" modes)
    const prefixInfo = useMemo(() => parseLogPrefix(entry.content), [entry.content]);

    // Get content without prefix for ANSI processing
    const contentToProcess = prefixInfo ? prefixInfo.rest : entry.content;

    // Convert ANSI codes in content
    const normalizedContent = normalizeAnsiCodes(contentToProcess);
    let htmlContent = converter.toHtml(normalizedContent);

    // If searching and this line matches, highlight the matches while preserving ANSI colors
    if (searchTerm && searchRegex && entry.isMatch) {
        const strippedContent = stripAnsiCodes(normalizedContent);
        htmlContent = highlightMatchesInHtml(htmlContent, strippedContent, searchRegex);
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
            className={`flex ${entry.isMatch ? 'bg-yellow-500/10' : ''} ${wrapLines ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'}`}
        >
            {showTimestamps && entry.timestamp && (
                <span className="text-gray-500 select-none mr-2 shrink-0">
                    {entry.timestamp}
                </span>
            )}
            {renderPrefix()}
            <span dangerouslySetInnerHTML={{ __html: htmlContent }} />
        </div>
    );
});

/**
 * Spinner component for loading states.
 */
export function Spinner({ className = "w-4 h-4" }) {
    return (
        <svg className={`${className} animate-spin`} fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
        </svg>
    );
}
