import React, { useState } from 'react';
import {
    ExclamationTriangleIcon,
    ArrowPathIcon,
    ClipboardIcon,
    ClipboardDocumentCheckIcon,
    CommandLineIcon
} from '@heroicons/react/24/outline';

const providerIcons = {
    aws: '☁️',
    azure: '☁️',
    gcloud: '☁️',
    network: '🔌',
    unknown: '⚠️'
};

export default function ConnectionError({ error, onRetry, isRetrying }) {
    const [copied, setCopied] = useState(false);

    if (!error) return null;

    const handleCopy = async () => {
        const text = `${error.title}\n${error.message}\n\nRaw error: ${error.raw}`;
        try {
            await navigator.clipboard.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <div className="flex-1 flex items-center justify-center bg-background p-8">
            <div className="max-w-lg w-full">
                <div className="bg-surface border border-red-500/30 rounded-lg shadow-lg overflow-hidden">
                    {/* Header */}
                    <div className="bg-red-500/10 px-6 py-4 border-b border-red-500/20">
                        <div className="flex items-center gap-3">
                            <div className="text-2xl">{providerIcons[error.provider] || providerIcons.unknown}</div>
                            <div>
                                <h2 className="text-lg font-semibold text-red-400">
                                    {error.title}
                                </h2>
                                <p className="text-sm text-gray-400">
                                    Failed to connect to Kubernetes cluster
                                </p>
                            </div>
                        </div>
                    </div>

                    {/* Body */}
                    <div className="px-6 py-4 space-y-4">
                        <p className="text-gray-300">
                            {error.message}
                        </p>

                        {error.suggestion && (
                            <div className="bg-blue-500/10 border border-blue-500/20 rounded-md p-3">
                                <div className="flex items-start gap-2">
                                    <CommandLineIcon className="h-5 w-5 text-blue-400 shrink-0 mt-0.5" />
                                    <div>
                                        <p className="text-sm font-medium text-blue-400">Suggestion</p>
                                        <p className="text-sm text-gray-300 mt-1">{error.suggestion}</p>
                                    </div>
                                </div>
                            </div>
                        )}

                        {/* Raw error details */}
                        <details className="text-sm">
                            <summary className="cursor-pointer text-gray-400 hover:text-gray-300 flex items-center gap-2">
                                <ExclamationTriangleIcon className="h-4 w-4" />
                                Show raw error
                            </summary>
                            <div className="mt-2 p-3 bg-black/30 rounded-md font-mono text-xs text-gray-400 break-all">
                                {error.raw}
                            </div>
                        </details>
                    </div>

                    {/* Actions */}
                    <div className="px-6 py-4 bg-black/20 border-t border-gray-700/50 flex items-center justify-between">
                        <button
                            onClick={handleCopy}
                            className="flex items-center gap-2 px-3 py-2 text-sm text-gray-400 hover:text-white hover:bg-white/10 rounded-md transition-colors"
                        >
                            {copied ? (
                                <>
                                    <ClipboardDocumentCheckIcon className="h-4 w-4 text-green-400" />
                                    <span className="text-green-400">Copied</span>
                                </>
                            ) : (
                                <>
                                    <ClipboardIcon className="h-4 w-4" />
                                    <span>Copy error</span>
                                </>
                            )}
                        </button>

                        <button
                            onClick={onRetry}
                            disabled={isRetrying}
                            className="flex items-center gap-2 px-4 py-2 bg-primary hover:bg-primary-hover disabled:opacity-50 disabled:cursor-not-allowed text-white rounded-md transition-colors"
                        >
                            <ArrowPathIcon className={`h-4 w-4 ${isRetrying ? 'animate-spin' : ''}`} />
                            {isRetrying ? 'Retrying...' : 'Retry Connection'}
                        </button>
                    </div>
                </div>

                {/* Additional help */}
                <p className="mt-4 text-center text-sm text-gray-500">
                    Make sure your kubeconfig is valid and the cluster is accessible.
                </p>
            </div>
        </div>
    );
}
