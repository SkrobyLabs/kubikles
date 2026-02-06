import React, { useState, useRef, useEffect, useMemo } from 'react';
import { Squares2X2Icon, MagnifyingGlassIcon } from '@heroicons/react/24/outline';

/**
 * Column Configurator Component
 *
 * Provides a popover button to configure which columns are visible in a table.
 * Includes "Show All" and "Reset to Default" options.
 */
export default function ColumnConfigurator({
    columns,
    hiddenColumns,
    onToggleColumn,
    onShowAll,
    onResetDefaults,
    defaultHiddenColumns = new Set<any>(),
}: { columns: any; hiddenColumns: any; onToggleColumn: any; onShowAll: any; onResetDefaults: any; defaultHiddenColumns?: any }) {
    const [isOpen, setIsOpen] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const containerRef = useRef<any>(null);
    const searchInputRef = useRef<any>(null);

    // Filter out special columns (selection, column selector)
    const configurableColumns = columns.filter(
        (col: any) => !col.isColumnSelector && !col.isSelectionColumn
    );

    // Filter columns by search term
    const filteredColumns = useMemo(() => {
        if (!searchTerm.trim()) return configurableColumns;
        const term = searchTerm.toLowerCase();
        return configurableColumns.filter((col: any) =>
            col.label.toLowerCase().includes(term) ||
            col.key.toLowerCase().includes(term)
        );
    }, [configurableColumns, searchTerm]);

    // Count visible columns
    const visibleCount = configurableColumns.filter((col: any) => !hiddenColumns.has(col.key)).length;
    const totalCount = configurableColumns.length;

    // Close dropdown when clicking outside
    useEffect(() => {
        const handleClickOutside = (e: any) => {
            if (containerRef.current && !(containerRef.current as any).contains(e.target)) {
                setIsOpen(false);
                setSearchTerm('');
            }
        };
        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [isOpen]);

    // Focus search input when opening (only if many columns)
    useEffect(() => {
        if (isOpen && configurableColumns.length > 8 && searchInputRef.current) {
            (searchInputRef.current as any).focus();
        }
    }, [isOpen, configurableColumns.length]);

    // Close on Escape key
    useEffect(() => {
        const handleKeyDown = (e: any) => {
            if (e.key === 'Escape' && isOpen) {
                setIsOpen(false);
                setSearchTerm('');
            }
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen]);

    return (
        <div className="relative no-drag" ref={containerRef}>
            <button
                onClick={() => setIsOpen(!isOpen)}
                className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                title="Configure visible columns"
            >
                <Squares2X2Icon className="w-4 h-4" />
            </button>

            {isOpen && (
                <div className="absolute right-0 top-full mt-1 w-56 bg-surface border border-border rounded-lg shadow-xl z-50">
                    {/* Header */}
                    <div className="px-3 py-2 border-b border-border">
                        <div className="text-sm font-medium text-text">Columns</div>
                        <div className="text-xs text-gray-500">{visibleCount} of {totalCount} visible</div>
                    </div>

                    {/* Search input */}
                    <div className="px-2 py-2 border-b border-border">
                        <div className="relative">
                            <MagnifyingGlassIcon className="w-3.5 h-3.5 text-gray-500 absolute left-2 top-1/2 -translate-y-1/2" />
                            <input
                                ref={searchInputRef}
                                type="text"
                                value={searchTerm}
                                onChange={(e: any) => setSearchTerm(e.target.value)}
                                placeholder="Search columns..."
                                className="w-full pl-7 pr-2 py-1 text-xs bg-background border border-border rounded focus:outline-none focus:border-primary"
                                autoComplete="off"
                            />
                        </div>
                    </div>

                    {/* Column list */}
                    <div className="max-h-64 overflow-auto py-1">
                        {filteredColumns.length === 0 ? (
                            <div className="px-3 py-2 text-xs text-gray-500 text-center">
                                No matching columns
                            </div>
                        ) : filteredColumns.map((col: any) => {
                            const isVisible = !hiddenColumns.has(col.key);
                            const isDefaultHidden = defaultHiddenColumns.has(col.key);
                            return (
                                <label
                                    key={col.key}
                                    className="flex items-center gap-2 px-3 py-1.5 text-xs hover:bg-white/5 cursor-pointer"
                                >
                                    <input
                                        type="checkbox"
                                        checked={isVisible}
                                        onChange={() => onToggleColumn(col.key)}
                                        className="w-3 h-3 rounded border-gray-600 bg-background text-primary focus:ring-primary"
                                    />
                                    <span className={`flex-1 truncate ${isVisible ? 'text-gray-200' : 'text-gray-500'}`}>
                                        {col.label}
                                    </span>
                                    {isDefaultHidden && isVisible && (
                                        <span className="text-[10px] text-gray-600">custom</span>
                                    )}
                                </label>
                            );
                        })}
                    </div>

                    {/* Actions */}
                    <div className="px-3 py-2 border-t border-border flex items-center gap-2">
                        <button
                            onClick={() => {
                                onShowAll();
                                setIsOpen(false);
                            }}
                            className="flex-1 px-2 py-1 text-xs text-gray-400 hover:text-white bg-background hover:bg-white/10 border border-border rounded transition-colors"
                        >
                            Show All
                        </button>
                        <button
                            onClick={() => {
                                onResetDefaults();
                                setIsOpen(false);
                            }}
                            className="flex-1 px-2 py-1 text-xs text-gray-400 hover:text-white bg-background hover:bg-white/10 border border-border rounded transition-colors"
                        >
                            Reset
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}
