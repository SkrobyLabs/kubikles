import React, { useState, useEffect, useCallback, useRef } from 'react';
import { XMarkIcon, ChevronDownIcon, ChevronRightIcon, ClipboardIcon, CheckIcon, ArrowDownTrayIcon, ExclamationTriangleIcon, ArrowPathIcon } from '@heroicons/react/24/outline';

/**
 * BulkActionModal - Modal for bulk action confirmation and progress
 *
 * States:
 * - confirmation: Shows list of resources and asks for confirmation
 * - inProgress: Shows progress bar and status updates
 * - complete: Shows final status with expandable details
 *
 * @param {Object} props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {Function} props.onClose - Called when modal is closed
 * @param {string} props.action - Action type ('delete', 'restart', etc.)
 * @param {string} props.actionLabel - Human-readable action label (e.g., 'Delete', 'Restart')
 * @param {Array} props.items - Array of items to act on
 * @param {Function} props.onConfirm - Called when action is confirmed, receives items array
 * @param {Function} props.onExportYaml - Optional callback to export YAML backup before action
 * @param {Object} props.progress - Progress state { current, total, status: 'idle'|'inProgress'|'complete', results: [{name, namespace, success, message}] }
 */
export default function BulkActionModal({
    isOpen,
    onClose,
    action,
    actionLabel,
    items = [],
    onConfirm,
    onExportYaml,
    progress = { current: 0, total: 0, status: 'idle', results: [] },
}) {
    const [detailsExpanded, setDetailsExpanded] = useState(false);
    const [copied, setCopied] = useState(false);
    const [backupInProgress, setBackupInProgress] = useState(false);
    const modalRef = useRef(null);

    // Get action-specific styles
    const getActionStyles = () => {
        switch (action) {
            case 'delete':
                return {
                    buttonClass: 'bg-red-600 hover:bg-red-700 text-white',
                    iconClass: 'text-red-400',
                    progressClass: 'bg-red-500',
                };
            case 'restart':
                return {
                    buttonClass: 'bg-yellow-600 hover:bg-yellow-700 text-white',
                    iconClass: 'text-yellow-400',
                    progressClass: 'bg-yellow-500',
                };
            default:
                return {
                    buttonClass: 'bg-primary hover:bg-primary/90 text-white',
                    iconClass: 'text-primary',
                    progressClass: 'bg-primary',
                };
        }
    };

    const styles = getActionStyles();

    // Handle keyboard events
    useEffect(() => {
        if (!isOpen) return;

        const handleKeyDown = (e) => {
            if (e.key === 'Escape') {
                onClose();
            } else if (e.key === 'Delete' && progress.status === 'idle') {
                onConfirm(items);
            }
            // Enter does nothing (as per user request)
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onClose, onConfirm, items, progress.status]);

    // Focus trap
    useEffect(() => {
        if (isOpen && modalRef.current) {
            modalRef.current.focus();
        }
    }, [isOpen]);

    // Copy results to clipboard
    const handleCopyResults = useCallback(() => {
        const text = progress.results
            .map(r => `${r.success ? 'OK' : 'FAIL'} ${r.namespace}/${r.name}${r.message ? ': ' + r.message : ''}`)
            .join('\n');
        navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    }, [progress.results]);

    // Reset state when modal opens
    useEffect(() => {
        if (isOpen && progress.status === 'idle') {
            setDetailsExpanded(false);
            setBackupInProgress(false);
        }
    }, [isOpen, progress.status]);

    // Handle backup with loading state
    const handleBackup = useCallback(async () => {
        if (!onExportYaml || backupInProgress) return;
        setBackupInProgress(true);
        try {
            await onExportYaml(items);
        } finally {
            setBackupInProgress(false);
        }
    }, [onExportYaml, items, backupInProgress]);

    if (!isOpen) return null;

    const progressPercent = progress.total > 0 ? Math.round((progress.current / progress.total) * 100) : 0;
    const successCount = progress.results.filter(r => r.success).length;
    const failCount = progress.results.filter(r => !r.success).length;

    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div
                ref={modalRef}
                tabIndex={-1}
                className="bg-surface border border-border rounded-lg shadow-xl w-full max-w-lg max-h-[80vh] flex flex-col outline-none"
            >
                {/* Header */}
                <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
                    <h2 className="text-lg font-semibold text-text">
                        {progress.status === 'complete'
                            ? `${actionLabel} Complete`
                            : progress.status === 'inProgress'
                            ? `${actionLabel}ing...`
                            : `${actionLabel} ${items.length} ${items.length === 1 ? 'Resource' : 'Resources'}`}
                    </h2>
                    <button
                        onClick={onClose}
                        className="p-1 hover:bg-white/10 rounded transition-colors"
                    >
                        <XMarkIcon className="h-5 w-5 text-gray-400" />
                    </button>
                </div>

                {/* Content */}
                <div className="flex-1 overflow-auto p-4">
                    {progress.status === 'idle' && (
                        <>
                            {/* Warning message */}
                            <div className="flex items-start gap-3 mb-4 p-3 bg-yellow-500/10 border border-yellow-500/30 rounded-lg">
                                <ExclamationTriangleIcon className="h-5 w-5 text-yellow-400 shrink-0 mt-0.5" />
                                <div className="text-sm text-yellow-200">
                                    {action === 'delete'
                                        ? 'This action cannot be undone. The selected resources will be permanently deleted.'
                                        : `You are about to ${action} the following resources:`}
                                </div>
                            </div>

                            {/* Resource list */}
                            <div className="text-sm text-gray-400 mb-2">
                                Affected resources ({items.length}):
                            </div>
                            <div className="bg-background rounded-lg border border-border max-h-48 overflow-auto">
                                {items.map((item, idx) => (
                                    <div
                                        key={item.metadata?.uid || idx}
                                        className="px-3 py-2 border-b border-border last:border-b-0 text-sm"
                                    >
                                        <span className="text-gray-500">{item.metadata?.namespace}/</span>
                                        <span className="text-text">{item.metadata?.name}</span>
                                    </div>
                                ))}
                            </div>
                        </>
                    )}

                    {progress.status === 'inProgress' && (
                        <>
                            {/* Progress bar */}
                            <div className="mb-4">
                                <div className="flex justify-between text-sm text-gray-400 mb-1">
                                    <span>Progress</span>
                                    <span>{progress.current} / {progress.total}</span>
                                </div>
                                <div className="h-2 bg-background rounded-full overflow-hidden">
                                    <div
                                        className={`h-full ${styles.progressClass} transition-all duration-300`}
                                        style={{ width: `${progressPercent}%` }}
                                    />
                                </div>
                            </div>

                            {/* Status updates - collapsible */}
                            <button
                                onClick={() => setDetailsExpanded(!detailsExpanded)}
                                className="flex items-center gap-1 text-sm text-gray-400 hover:text-text transition-colors mb-2"
                            >
                                {detailsExpanded ? (
                                    <ChevronDownIcon className="h-4 w-4" />
                                ) : (
                                    <ChevronRightIcon className="h-4 w-4" />
                                )}
                                <span>Details ({progress.results.length})</span>
                            </button>

                            {detailsExpanded && (
                                <div className="bg-background rounded-lg border border-border max-h-48 overflow-auto">
                                    {progress.results.map((result, idx) => (
                                        <div
                                            key={idx}
                                            className="px-3 py-2 border-b border-border last:border-b-0 text-sm flex items-start gap-2"
                                        >
                                            <span className={result.success ? 'text-green-400' : 'text-red-400'}>
                                                {result.success ? 'OK' : 'FAIL'}
                                            </span>
                                            <span className="text-text flex-1">
                                                {result.namespace}/{result.name}
                                                {result.message && (
                                                    <span className="text-gray-500 ml-2">- {result.message}</span>
                                                )}
                                            </span>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </>
                    )}

                    {progress.status === 'complete' && (
                        <>
                            {/* Summary */}
                            <div className="flex items-center gap-4 mb-4">
                                {successCount > 0 && (
                                    <div className="flex items-center gap-2">
                                        <div className="w-3 h-3 rounded-full bg-green-500" />
                                        <span className="text-sm text-gray-300">{successCount} succeeded</span>
                                    </div>
                                )}
                                {failCount > 0 && (
                                    <div className="flex items-center gap-2">
                                        <div className="w-3 h-3 rounded-full bg-red-500" />
                                        <span className="text-sm text-gray-300">{failCount} failed</span>
                                    </div>
                                )}
                            </div>

                            {/* Results - collapsible */}
                            <div className="flex items-center justify-between mb-2">
                                <button
                                    onClick={() => setDetailsExpanded(!detailsExpanded)}
                                    className="flex items-center gap-1 text-sm text-gray-400 hover:text-text transition-colors"
                                >
                                    {detailsExpanded ? (
                                        <ChevronDownIcon className="h-4 w-4" />
                                    ) : (
                                        <ChevronRightIcon className="h-4 w-4" />
                                    )}
                                    <span>Details ({progress.results.length})</span>
                                </button>

                                {/* Copy button */}
                                <button
                                    onClick={handleCopyResults}
                                    className="flex items-center gap-1 text-sm text-gray-400 hover:text-text transition-colors"
                                    title="Copy results to clipboard"
                                >
                                    {copied ? (
                                        <>
                                            <CheckIcon className="h-4 w-4 text-green-400" />
                                            <span className="text-green-400">Copied</span>
                                        </>
                                    ) : (
                                        <>
                                            <ClipboardIcon className="h-4 w-4" />
                                            <span>Copy</span>
                                        </>
                                    )}
                                </button>
                            </div>

                            {detailsExpanded && (
                                <div className="bg-background rounded-lg border border-border max-h-48 overflow-auto">
                                    {progress.results.map((result, idx) => (
                                        <div
                                            key={idx}
                                            className="px-3 py-2 border-b border-border last:border-b-0 text-sm flex items-start gap-2"
                                        >
                                            <span className={result.success ? 'text-green-400' : 'text-red-400'}>
                                                {result.success ? 'OK' : 'FAIL'}
                                            </span>
                                            <span className="text-text flex-1">
                                                {result.namespace}/{result.name}
                                                {result.message && (
                                                    <span className="text-gray-500 ml-2">- {result.message}</span>
                                                )}
                                            </span>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </>
                    )}
                </div>

                {/* Footer */}
                <div className="flex items-center justify-between px-4 py-3 border-t border-border shrink-0">
                    {progress.status === 'idle' ? (
                        <>
                            <div className="flex items-center gap-2">
                                {onExportYaml && (
                                    <button
                                        onClick={handleBackup}
                                        disabled={backupInProgress}
                                        className={`flex items-center gap-1.5 px-3 py-1.5 text-sm rounded transition-colors ${
                                            backupInProgress
                                                ? 'text-gray-500 cursor-not-allowed'
                                                : 'text-gray-300 hover:text-text hover:bg-white/10'
                                        }`}
                                        title="Download YAML backup before action"
                                    >
                                        {backupInProgress ? (
                                            <>
                                                <ArrowPathIcon className="h-4 w-4 animate-spin" />
                                                <span>Downloading...</span>
                                            </>
                                        ) : (
                                            <>
                                                <ArrowDownTrayIcon className="h-4 w-4" />
                                                <span>Backup YAML</span>
                                            </>
                                        )}
                                    </button>
                                )}
                            </div>
                            <div className="flex items-center gap-2">
                                <button
                                    onClick={onClose}
                                    className="px-4 py-1.5 text-sm text-gray-300 hover:text-text hover:bg-white/10 rounded transition-colors"
                                >
                                    Cancel
                                </button>
                                <button
                                    onClick={() => onConfirm(items)}
                                    className={`px-4 py-1.5 text-sm rounded transition-colors ${styles.buttonClass}`}
                                >
                                    {actionLabel}
                                </button>
                            </div>
                        </>
                    ) : progress.status === 'inProgress' ? (
                        <div className="w-full text-center text-sm text-gray-400">
                            Please wait...
                        </div>
                    ) : (
                        <div className="w-full flex justify-end">
                            <button
                                onClick={onClose}
                                className="px-4 py-1.5 text-sm bg-surface border border-border hover:bg-white/10 rounded transition-colors text-text"
                            >
                                Close
                            </button>
                        </div>
                    )}
                </div>
            </div>
        </div>
    );
}
