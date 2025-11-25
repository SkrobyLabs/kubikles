import React, { useState, useEffect, useRef } from 'react';
import { ChevronDownIcon, MagnifyingGlassIcon } from '@heroicons/react/24/outline';

export default function SearchSelect({ options, value, onChange, placeholder = "Select...", className = "", multiSelect = false }) {
    const [isOpen, setIsOpen] = useState(false);
    const [searchTerm, setSearchTerm] = useState("");
    const wrapperRef = useRef(null);
    const inputRef = useRef(null);

    // Helper to get display label for an option
    const getDisplayLabel = (option) => {
        return option === '' ? 'All Namespaces' : option;
    };

    // Helper to get display value for multi-select
    const getMultiSelectDisplay = () => {
        if (!value || value.length === 0) {
            return placeholder;
        }
        // Check for "all" marker
        if (value.includes('*')) {
            return 'All Namespaces';
        }
        if (value.length === 1) {
            return getDisplayLabel(value[0]);
        }
        return `${value.length} namespaces selected`;
    };

    useEffect(() => {
        function handleClickOutside(event) {
            if (wrapperRef.current && !wrapperRef.current.contains(event.target)) {
                setIsOpen(false);
            }
        }
        document.addEventListener("mousedown", handleClickOutside);
        return () => {
            document.removeEventListener("mousedown", handleClickOutside);
        };
    }, [wrapperRef]);

    useEffect(() => {
        if (isOpen && inputRef.current) {
            inputRef.current.focus();
        }
        if (!isOpen) {
            setSearchTerm("");
        }
    }, [isOpen]);

    const isOptionSelected = (option) => {
        if (!multiSelect) {
            return option === value;
        }
        return Array.isArray(value) && value.includes(option);
    };

    const filteredOptions = options
        .filter(option =>
            getDisplayLabel(option).toLowerCase().includes(searchTerm.toLowerCase())
        )
        .sort((a, b) => {
            // Sort selected items first
            const aSelected = isOptionSelected(a);
            const bSelected = isOptionSelected(b);
            if (aSelected && !bSelected) return -1;
            if (!aSelected && bSelected) return 1;
            // If both selected or both unselected, maintain original order (alphabetically via locale compare)
            return getDisplayLabel(a).localeCompare(getDisplayLabel(b));
        });

    const handleOptionClick = (option) => {
        if (!multiSelect) {
            onChange(option);
            setIsOpen(false);
            return;
        }

        // Multi-select logic
        const currentValue = Array.isArray(value) ? value : [];
        const isSelected = currentValue.includes(option);

        // If "All Namespaces" (*) is selected and user clicks an individual namespace,
        // replace the selection with just that namespace
        if (currentValue.includes('*') && option !== '*') {
            onChange([option]);
            return;
        }

        if (isSelected) {
            onChange(currentValue.filter(v => v !== option));
        } else {
            onChange([...currentValue, option]);
        }
    };

    return (
        <div className={`relative ${className}`} ref={wrapperRef}>
            <button
                onClick={() => setIsOpen(!isOpen)}
                className="w-full flex items-center justify-between px-3 py-2 bg-surface border border-border rounded text-sm text-text hover:border-primary focus:outline-none focus:border-primary transition-colors"
            >
                <span className="truncate">
                    {multiSelect ? getMultiSelectDisplay() : (value !== undefined && value !== null ? getDisplayLabel(value) : placeholder)}
                </span>
                <ChevronDownIcon className="h-4 w-4 text-gray-400 ml-2 shrink-0" />
            </button>

            {isOpen && (
                <div className="absolute z-50 w-full mt-1 bg-surface border border-border rounded shadow-lg max-h-60 flex flex-col">
                    <div className="p-2 border-b border-border sticky top-0 bg-surface">
                        <div className="relative">
                            <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-2 top-1/2 transform -translate-y-1/2" />
                            <input
                                ref={inputRef}
                                type="text"
                                className="w-full bg-background border border-border rounded pl-8 pr-2 py-1 text-sm text-text focus:outline-none focus:border-primary"
                                placeholder="Search..."
                                value={searchTerm}
                                onChange={(e) => setSearchTerm(e.target.value)}
                                onClick={(e) => e.stopPropagation()}
                                autoComplete="off"
                                autoCorrect="off"
                                spellCheck="false"
                            />
                        </div>
                        {multiSelect && !searchTerm && (
                            <button
                                className="w-full mt-2 px-3 py-2 text-sm bg-primary/10 text-primary rounded hover:bg-primary/20 transition-colors flex items-center gap-2"
                                onClick={() => {
                                    const allNamespaces = options.filter(opt => opt !== '');
                                    const currentValue = Array.isArray(value) ? value : [];

                                    // Check if special "all" marker is present
                                    const hasAllMarker = currentValue.includes('*');
                                    const allSelected = hasAllMarker || (allNamespaces.length > 0 && allNamespaces.every(ns => currentValue.includes(ns)));
                                    const noneSelected = currentValue.length === 0 || !currentValue.some(v => allNamespaces.includes(v) || v === '*');

                                    if (allSelected) {
                                        // All selected - unselect all
                                        onChange([]);
                                    } else if (noneSelected) {
                                        // None selected - select all using special marker
                                        onChange(['*']);
                                    } else {
                                        // Mixed state - unselect all (safer option)
                                        onChange([]);
                                    }
                                }}
                            >
                                <input
                                    type="checkbox"
                                    checked={(() => {
                                        const allNamespaces = options.filter(opt => opt !== '');
                                        const currentValue = Array.isArray(value) ? value : [];
                                        const hasAllMarker = currentValue.includes('*');
                                        return hasAllMarker || (allNamespaces.length > 0 && allNamespaces.every(ns => currentValue.includes(ns)));
                                    })()}
                                    ref={(el) => {
                                        if (el) {
                                            const allNamespaces = options.filter(opt => opt !== '');
                                            const currentValue = Array.isArray(value) ? value : [];
                                            const hasAllMarker = currentValue.includes('*');
                                            const someSelected = currentValue.some(v => allNamespaces.includes(v) || v === '*');
                                            const allSelected = hasAllMarker || (allNamespaces.length > 0 && allNamespaces.every(ns => currentValue.includes(ns)));
                                            el.indeterminate = someSelected && !allSelected;
                                        }
                                    }}
                                    onChange={() => {}}
                                    className="h-4 w-4 rounded border-border text-primary focus:ring-primary"
                                />
                                <span>All Namespaces ({options.filter(opt => opt !== '').length})</span>
                            </button>
                        )}
                        {multiSelect && searchTerm && filteredOptions.filter(opt => opt !== '').length > 0 && (
                            <button
                                className="w-full mt-2 px-3 py-2 text-sm bg-primary/10 text-primary rounded hover:bg-primary/20 transition-colors flex items-center gap-2"
                                onClick={() => {
                                    const matchingNamespaces = filteredOptions.filter(opt => opt !== '');
                                    const currentValue = Array.isArray(value) ? value : [];
                                    const allMatchingSelected = matchingNamespaces.every(ns => currentValue.includes(ns));
                                    const noneMatchingSelected = !currentValue.some(v => matchingNamespaces.includes(v));

                                    if (allMatchingSelected) {
                                        // All matching selected - unselect all matching
                                        onChange(currentValue.filter(v => !matchingNamespaces.includes(v)));
                                    } else if (noneMatchingSelected) {
                                        // None matching selected - select all matching
                                        const newSelections = [...new Set([...currentValue, ...matchingNamespaces])];
                                        onChange(newSelections);
                                    } else {
                                        // Mixed state - unselect all matching (safer option)
                                        onChange(currentValue.filter(v => !matchingNamespaces.includes(v)));
                                    }
                                }}
                            >
                                <input
                                    type="checkbox"
                                    checked={(() => {
                                        const matchingNamespaces = filteredOptions.filter(opt => opt !== '');
                                        const currentValue = Array.isArray(value) ? value : [];
                                        return matchingNamespaces.length > 0 && matchingNamespaces.every(ns => currentValue.includes(ns));
                                    })()}
                                    ref={(el) => {
                                        if (el) {
                                            const matchingNamespaces = filteredOptions.filter(opt => opt !== '');
                                            const currentValue = Array.isArray(value) ? value : [];
                                            const someSelected = currentValue.some(v => matchingNamespaces.includes(v));
                                            const allSelected = matchingNamespaces.length > 0 && matchingNamespaces.every(ns => currentValue.includes(ns));
                                            el.indeterminate = someSelected && !allSelected;
                                        }
                                    }}
                                    onChange={() => {}}
                                    className="h-4 w-4 rounded border-border text-primary focus:ring-primary"
                                />
                                <span>Select all matching ({filteredOptions.filter(opt => opt !== '').length})</span>
                            </button>
                        )}
                    </div>
                    <div className="overflow-y-auto flex-1">
                        {filteredOptions.filter(opt => opt !== '').length > 0 ? (
                            filteredOptions.filter(opt => opt !== '').map((option) => (
                                <div
                                    key={option || '__all__'}
                                    className={`px-3 py-2 text-sm cursor-pointer hover:bg-primary/10 flex items-center gap-2 ${isOptionSelected(option) ? 'text-primary font-medium' : 'text-text'}`}
                                    onClick={() => handleOptionClick(option)}
                                >
                                    {multiSelect && (
                                        <input
                                            type="checkbox"
                                            checked={isOptionSelected(option)}
                                            onChange={() => {}}
                                            className="h-4 w-4 rounded border-border text-primary focus:ring-primary"
                                        />
                                    )}
                                    <span className="flex-1">{getDisplayLabel(option)}</span>
                                </div>
                            ))
                        ) : (
                            <div className="px-3 py-2 text-sm text-gray-500 text-center">
                                No results found
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
