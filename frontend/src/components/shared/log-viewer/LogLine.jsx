import React from 'react';
import { converter, normalizeAnsiCodes, stripAnsiCodes, highlightMatchesInHtml } from './logUtils';

/**
 * Renders a single log line with ANSI color support and search highlighting.
 */
export function LogLine({
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

    // Convert ANSI codes in content
    const normalizedContent = normalizeAnsiCodes(entry.content);
    let htmlContent = converter.toHtml(normalizedContent);

    // If searching and this line matches, highlight the matches while preserving ANSI colors
    if (searchTerm && searchRegex && entry.isMatch) {
        const strippedContent = stripAnsiCodes(normalizedContent);
        htmlContent = highlightMatchesInHtml(htmlContent, strippedContent, searchRegex);
    }

    return (
        <div className={`flex ${entry.isMatch ? 'bg-yellow-500/10' : ''} ${wrapLines ? 'whitespace-pre-wrap break-all' : 'whitespace-pre'}`}>
            {showTimestamps && entry.timestamp && (
                <span className="text-gray-500 select-none mr-2 shrink-0">
                    {entry.timestamp}
                </span>
            )}
            <span dangerouslySetInnerHTML={{ __html: htmlContent }} />
        </div>
    );
}

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
