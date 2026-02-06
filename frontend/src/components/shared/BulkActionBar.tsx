import React from 'react';
import { XMarkIcon, TrashIcon, ArrowPathIcon, ArrowDownTrayIcon } from '@heroicons/react/24/outline';

/**
 * BulkActionBar - Contextual action bar that appears when items are selected
 *
 * @param {Object} props
 * @param {number} props.selectedCount - Number of selected items
 * @param {Function} props.onClearSelection - Called when clear selection button is clicked
 * @param {Function} props.onDelete - Called when delete button is clicked (optional)
 * @param {Function} props.onRestart - Called when restart button is clicked (optional, for workloads)
 * @param {Function} props.onExportYaml - Called when export YAML button is clicked (optional)
 * @param {string} props.resourceType - Type of resource (e.g., 'pods', 'deployments')
 * @param {string} props.position - Position of the bar ('top' or 'bottom')
 */
export default function BulkActionBar({
    selectedCount,
    onClearSelection,
    onDelete,
    onRestart,
    onExportYaml,
    resourceType = 'items',
    position = 'top',
}: any) {
    if (selectedCount === 0) return null;

    // Format the count display
    const countDisplay = selectedCount === 1
        ? `1 ${resourceType.replace(/s$/, '')} selected`
        : `${selectedCount} ${resourceType} selected`;

    const isBottom = position === 'bottom';

    return (
        <div className={`h-12 flex items-center justify-between px-4 bg-primary/10 shrink-0 ${
            isBottom
                ? 'border-t border-primary/30 animate-in slide-in-from-bottom-2 duration-200'
                : 'border-b border-primary/30 animate-in slide-in-from-top-2 duration-200'
        }`}>
            <div className="flex items-center gap-3">
                {/* Clear selection button */}
                <button
                    onClick={onClearSelection}
                    className="p-1.5 hover:bg-white/10 rounded transition-colors"
                    title="Clear selection"
                >
                    <XMarkIcon className="h-5 w-5 text-gray-300" />
                </button>

                {/* Selection count */}
                <span className="text-sm text-text font-medium">
                    {countDisplay}
                </span>
            </div>

            <div className="flex items-center gap-2">
                {/* Export YAML button */}
                {onExportYaml && (
                    <button
                        onClick={onExportYaml}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-300 hover:text-text hover:bg-white/10 rounded transition-colors"
                        title="Export selected resources as YAML"
                    >
                        <ArrowDownTrayIcon className="h-4 w-4" />
                        <span>Export YAML</span>
                    </button>
                )}

                {/* Restart button (for workloads) */}
                {onRestart && (
                    <button
                        onClick={onRestart}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-yellow-400 hover:text-yellow-300 hover:bg-yellow-500/10 rounded transition-colors"
                        title="Restart selected resources"
                    >
                        <ArrowPathIcon className="h-4 w-4" />
                        <span>Restart</span>
                    </button>
                )}

                {/* Delete button */}
                {onDelete && (
                    <button
                        onClick={onDelete}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-red-400 hover:text-red-300 hover:bg-red-500/10 rounded transition-colors"
                        title="Delete selected resources"
                    >
                        <TrashIcon className="h-4 w-4" />
                        <span>Delete</span>
                    </button>
                )}
            </div>
        </div>
    );
}
