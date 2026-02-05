import React, { useState } from 'react';
import { ClipboardDocumentIcon, FolderOpenIcon, CheckIcon } from '@heroicons/react/24/outline';

export default function ReadonlyField({ label, description, value, showCopy = true, showOpenFolder = false, onOpenFolder }) {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
        if (!value) return;
        try {
            // Quote the path if it contains spaces
            const textToCopy = value.includes(' ') ? `"${value}"` : value;
            await navigator.clipboard.writeText(textToCopy);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <div className="py-3 flex items-start justify-between gap-4">
            <div className="flex-1 min-w-0">
                <div className="flex items-center gap-2">
                    <span className="text-sm font-medium text-text">{label}</span>
                </div>
                {description && (
                    <p className="text-xs text-text-muted mt-0.5">{description}</p>
                )}
                <div className="mt-1.5 flex items-center gap-2">
                    <code className="text-xs text-text-muted bg-background px-2 py-1 rounded font-mono truncate max-w-md" title={value}>
                        {value || 'Not available'}
                    </code>
                    {value && showCopy && (
                        <button
                            onClick={handleCopy}
                            className="p-1 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                            title="Copy path"
                        >
                            {copied ? (
                                <CheckIcon className="w-4 h-4 text-green-400" />
                            ) : (
                                <ClipboardDocumentIcon className="w-4 h-4" />
                            )}
                        </button>
                    )}
                    {value && showOpenFolder && onOpenFolder && (
                        <button
                            onClick={onOpenFolder}
                            className="p-1 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                            title="Open folder"
                        >
                            <FolderOpenIcon className="w-4 h-4" />
                        </button>
                    )}
                </div>
            </div>
        </div>
    );
}
