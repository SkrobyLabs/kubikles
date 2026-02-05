import React from 'react';
import { ArrowsRightLeftIcon, XMarkIcon } from '@heroicons/react/24/outline';
import { useUI } from '../../context';
import { useK8s } from '../../context';

/**
 * Comparison menu items for resource action menus.
 * Usage: Include <ComparisonMenuItems kind="pod" namespace="default" name="my-pod" onAction={closeMenu} />
 * in any actions menu to add comparison functionality.
 */
export default function ComparisonMenuItems({ kind, namespace, name, onAction }) {
    const { comparisonSource, setComparisonSource, clearComparisonSource, compareWithSource } = useUI();
    const { currentContext } = useK8s();

    const isSource = comparisonSource &&
        comparisonSource.kind === kind &&
        comparisonSource.namespace === namespace &&
        comparisonSource.name === name &&
        comparisonSource.context === currentContext;

    // Check if source is from a different context
    const isCrossContext = comparisonSource && comparisonSource.context !== currentContext;

    const handleSetSource = (e) => {
        e.stopPropagation();
        setComparisonSource(kind, namespace, name);
        onAction?.();
    };

    const handleCompare = (e) => {
        e.stopPropagation();
        compareWithSource(kind, namespace, name);
        onAction?.();
    };

    const handleClear = (e) => {
        e.stopPropagation();
        clearComparisonSource();
        onAction?.();
    };

    return (
        <>
            {comparisonSource && !isSource && (
                <button
                    onClick={handleCompare}
                    className="w-full text-left px-4 py-2 text-sm text-purple-400 hover:bg-surface-hover flex flex-col gap-0.5"
                >
                    <span className="flex items-center gap-2">
                        <ArrowsRightLeftIcon className="h-4 w-4 shrink-0" />
                        Compare with:
                    </span>
                    <span className="pl-6 text-xs break-all">
                        {isCrossContext && <span className="text-purple-300">[{comparisonSource?.context}] </span>}
                        {comparisonSource?.kind}/{comparisonSource?.name}
                    </span>
                </button>
            )}
            {isSource ? (
                <button
                    onClick={handleClear}
                    className="w-full text-left px-4 py-2 text-sm text-gray-400 hover:bg-surface-hover flex items-center gap-2"
                >
                    <XMarkIcon className="h-4 w-4" />
                    Clear comparison source
                </button>
            ) : (
                <button
                    onClick={handleSetSource}
                    className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
                >
                    <ArrowsRightLeftIcon className="h-4 w-4" />
                    {comparisonSource ? 'Use this as source instead' : 'Set as comparison source'}
                </button>
            )}
        </>
    );
}
