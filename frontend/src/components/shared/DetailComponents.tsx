import React, { useState } from 'react';
import { CheckIcon, ClipboardDocumentIcon } from '@heroicons/react/24/outline';

/**
 * Copyable label component - displays a value that can be clicked to copy
 */
export const CopyableLabel = ({ value, copyValue, className = '' }: { value: any; copyValue?: any; className?: string }) => {
    const [copied, setCopied] = useState(false);

    const textToCopy = copyValue || value;

    const handleCopy = async (e: any) => {
        e.stopPropagation();
        if (!textToCopy) return;
        try {
            await navigator.clipboard.writeText(textToCopy);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err: any) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <button
            onClick={handleCopy}
            className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded border transition-colors cursor-pointer ${
                copied
                    ? 'bg-green-500/20 text-green-400 border-green-500/30'
                    : `bg-gray-500/10 hover:bg-gray-500/20 text-gray-300 border-gray-500/30 ${className}`
            }`}
            title={copied ? 'Copied!' : `Click to copy: ${textToCopy}`}
        >
            {copied ? (
                <>
                    <CheckIcon className="w-3 h-3" />
                    Copied
                </>
            ) : (
                value
            )}
        </button>
    );
};

/**
 * Copyable text block - larger text area for messages/content with copy button
 */
export const CopyableTextBlock = ({ value, maxLines = 10 }: { value: any; maxLines?: number }) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
        if (!value) return;
        try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err: any) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <div className="relative group">
            <pre
                className="text-sm text-gray-200 bg-background-dark rounded border border-border p-3 whitespace-pre-wrap break-words overflow-auto"
                style={{ maxHeight: `${maxLines * 1.5}rem` }}
            >
                {value || <span className="text-gray-500 italic">No content</span>}
            </pre>
            {value && (
                <button
                    onClick={handleCopy}
                    className={`absolute top-2 right-2 p-1.5 rounded transition-colors ${
                        copied
                            ? 'bg-green-500/20 text-green-400'
                            : 'bg-gray-700 text-gray-400 hover:text-white opacity-0 group-hover:opacity-100'
                    }`}
                    title={copied ? 'Copied!' : 'Copy to clipboard'}
                >
                    {copied ? (
                        <CheckIcon className="w-4 h-4" />
                    ) : (
                        <ClipboardDocumentIcon className="w-4 h-4" />
                    )}
                </button>
            )}
        </div>
    );
};

/**
 * Detail row component - displays a label/value pair
 * Memoized to prevent re-renders when parent updates with same props
 */
export const DetailRow = React.memo(({ label, value, children }: { label: string; value?: any; children?: React.ReactNode }) => (
    <div className="flex py-2 border-b border-border/50">
        <div className="w-32 text-xs font-medium text-gray-500 uppercase tracking-wider shrink-0">
            {label}
        </div>
        <div className="flex-1 text-sm text-gray-200">
            {children || value || <span className="text-gray-500">N/A</span>}
        </div>
    </div>
));

/**
 * Detail section component - groups related detail rows with a title
 * Memoized to prevent re-renders when parent updates with same props
 */
export const DetailSection = React.memo(({ title, children }: { title?: string; children?: React.ReactNode }) => (
    <div className="bg-surface rounded-lg border border-border p-4 mb-4">
        {title && (
            <h3 className="text-sm font-medium text-gray-300 mb-3 pb-2 border-b border-border">
                {title}
            </h3>
        )}
        {children}
    </div>
));

/**
 * Status badge component
 * Memoized to prevent re-renders when parent updates with same props
 */
export const StatusBadge = React.memo(({ status, variant = 'default' }: { status: any; variant?: string }) => {
    const variants: Record<string, string> = {
        success: 'bg-green-500/10 text-green-400 border-green-500/30',
        warning: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30',
        error: 'bg-red-500/10 text-red-400 border-red-500/30',
        info: 'bg-blue-500/10 text-blue-400 border-blue-500/30',
        default: 'bg-gray-500/10 text-gray-400 border-gray-500/30',
    };

    return (
        <span className={`px-2 py-0.5 text-xs rounded border ${variants[variant] || variants.default}`}>
            {status}
        </span>
    );
});

/**
 * Labels display component
 * Memoized to prevent re-renders when parent updates with same props
 */
export const LabelsDisplay = React.memo(({ labels, emptyText = 'None' }: { labels: any; emptyText?: string }) => {
    if (!labels || Object.keys(labels).length === 0) {
        return <span className="text-gray-500">{emptyText}</span>;
    }

    return (
        <div className="flex flex-wrap gap-1.5">
            {Object.entries(labels)
                .sort(([a], [b]) => a.localeCompare(b))
                .map(([key, value]) => (
                    <CopyableLabel key={key} value={`${key}=${value}`} />
                ))}
        </div>
    );
});

/**
 * Annotations display component (same as labels but with different styling)
 * Memoized to prevent re-renders when parent updates with same props
 */
export const AnnotationsDisplay = React.memo(({ annotations, emptyText = 'None' }: { annotations: any; emptyText?: string }) => {
    if (!annotations || Object.keys(annotations).length === 0) {
        return <span className="text-gray-500">{emptyText}</span>;
    }

    return (
        <div className="flex flex-wrap gap-1.5">
            {Object.entries(annotations)
                .sort(([a], [b]) => a.localeCompare(b))
                .map(([key, value]) => (
                    <CopyableLabel
                        key={key}
                        value={key.length > 40 ? `${key.substring(0, 40)}...` : key}
                        copyValue={`${key}=${value}`}
                        className="bg-purple-500/10 border-purple-500/30"
                    />
                ))}
        </div>
    );
});

/**
 * Resource count badge
 * Memoized to prevent re-renders when parent updates with same props
 */
export const ResourceCountBadge = React.memo(({ count, label, onClick }: { count: any; label: any; onClick?: any }) => {
    const Component = onClick ? 'button' : 'div';
    return (
        <Component
            onClick={onClick}
            className={`flex items-center gap-2 px-3 py-2 rounded-lg border ${
                onClick
                    ? 'border-border hover:border-primary/50 hover:bg-primary/10 cursor-pointer transition-colors'
                    : 'border-border bg-surface'
            }`}
        >
            <span className="text-lg font-semibold text-gray-200">{count}</span>
            <span className="text-xs text-gray-400">{label}</span>
        </Component>
    );
});
