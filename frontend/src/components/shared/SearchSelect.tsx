import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { ChevronDownIcon, MagnifyingGlassIcon } from '@heroicons/react/24/outline';

export default function SearchSelect({
    options,
    value,
    onChange,
    placeholder = "Select...",
    className = "",
    multiSelect = false,
    // For object options: provide these to extract value/label
    getOptionValue = null,  // (option) => string value
    getOptionLabel = null,  // (option) => string label for display
    renderOption = null,    // (option, isSelected) => ReactNode for custom rendering
    disabled = false,
    onOpen = null,          // Callback when dropdown opens
    preserveOrder = false,  // If true, don't sort alphabetically (preserve input order)
    searchable = true,      // If false, hide the search input (for simple dropdowns)
    // Multi-select customization
    multiSelectLabels = null, // { all: "All Items", count: (n) => `${n} items selected` }
}) {
    const [isOpen, setIsOpen] = useState(false);
    const [searchTerm, setSearchTerm] = useState("");
    const wrapperRef = useRef(null);
    const inputRef = useRef(null);

    // Helper to get the value from an option (for comparison)
    const getValueFromOption = (option) => {
        if (getOptionValue) return getOptionValue(option);
        return option;
    };

    // Helper to get display label for an option
    const getDisplayLabel = (option) => {
        if (getOptionLabel) return getOptionLabel(option);
        return option === '' ? 'All Namespaces' : String(option);
    };

    // Default labels for multi-select (namespace-centric for backward compatibility)
    const defaultLabels = { all: 'All Namespaces', count: (n) => `${n} namespaces selected` };
    const labels = multiSelectLabels || defaultLabels;

    // Helper to get display value for multi-select
    const getMultiSelectDisplay = () => {
        if (!value || value.length === 0) {
            return placeholder;
        }
        // Check for "all" marker
        if (value.includes('*')) {
            return labels.all;
        }
        if (value.length === 1) {
            return getDisplayLabel(value[0]);
        }
        return labels.count(value.length);
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

    const isOptionSelected = useCallback((option) => {
        const optionValue = getValueFromOption(option);
        if (!multiSelect) {
            return optionValue === value;
        }
        return Array.isArray(value) && value.includes(optionValue);
    }, [getValueFromOption, multiSelect, value]);

    // Memoize filtered/sorted options (computed once per search/value change)
    const filteredOptions = useMemo(() => {
        const lowerSearch = searchTerm.toLowerCase();
        const filtered = options.filter(option =>
            getDisplayLabel(option).toLowerCase().includes(lowerSearch)
        );

        // If preserveOrder is true, keep the original order (just filter, don't sort)
        if (preserveOrder) {
            return filtered;
        }

        // Default: sort with selected first, then alphabetically
        return filtered.sort((a, b) => {
            const aSelected = isOptionSelected(a);
            const bSelected = isOptionSelected(b);
            if (aSelected && !bSelected) return -1;
            if (!aSelected && bSelected) return 1;
            return getDisplayLabel(a).localeCompare(getDisplayLabel(b));
        });
    }, [options, searchTerm, isOptionSelected, getDisplayLabel, preserveOrder]);

    // Memoize "all namespaces" (non-empty options) - used in multiple places
    const allNamespaces = useMemo(() => options.filter(opt => opt !== ''), [options]);

    // Memoize non-empty filtered options - used in rendering
    const nonEmptyFilteredOptions = useMemo(
        () => filteredOptions.filter(opt => getValueFromOption(opt) !== ''),
        [filteredOptions, getValueFromOption]
    );

    // Memoize selection state for "All Namespaces" checkbox
    // Use Set for O(1) lookups instead of O(n) array.includes()
    const allSelectionState = useMemo(() => {
        const currentValue = Array.isArray(value) ? value : [];
        const currentValueSet = new Set(currentValue);
        const allNamespacesSet = new Set(allNamespaces);
        const hasAllMarker = currentValueSet.has('*');
        const allSelected = hasAllMarker || (allNamespaces.length > 0 && allNamespaces.every(ns => currentValueSet.has(ns)));
        const someSelected = currentValue.some(v => allNamespacesSet.has(v) || v === '*');
        return { allSelected, someSelected, indeterminate: someSelected && !allSelected };
    }, [value, allNamespaces]);

    // Memoize selection state for "Select all matching" checkbox (when searching)
    // Use Set for O(1) lookups instead of O(n) array.includes()
    const matchingSelectionState = useMemo(() => {
        const currentValue = Array.isArray(value) ? value : [];
        const currentValueSet = new Set(currentValue);
        const matchingNamespaces = nonEmptyFilteredOptions.map(opt => typeof opt === 'string' ? opt : getValueFromOption(opt));
        const matchingNamespacesSet = new Set(matchingNamespaces);
        const allSelected = matchingNamespaces.length > 0 && matchingNamespaces.every(ns => currentValueSet.has(ns));
        const someSelected = currentValue.some(v => matchingNamespacesSet.has(v));
        return { allSelected, someSelected, indeterminate: someSelected && !allSelected, matchingNamespaces };
    }, [value, nonEmptyFilteredOptions, getValueFromOption]);

    const handleOptionClick = (option) => {
        const optionValue = getValueFromOption(option);

        if (!multiSelect) {
            onChange(optionValue);
            setIsOpen(false);
            return;
        }

        // Multi-select logic
        const currentValue = Array.isArray(value) ? value : [];
        const isSelected = currentValue.includes(optionValue);

        // If "All Namespaces" (*) is selected and user clicks an individual namespace,
        // replace the selection with just that namespace
        if (currentValue.includes('*') && optionValue !== '*') {
            onChange([optionValue]);
            return;
        }

        if (isSelected) {
            onChange(currentValue.filter(v => v !== optionValue));
        } else {
            onChange([...currentValue, optionValue]);
        }
    };

    // Find the selected option object for display (when using object options)
    const getSelectedOptionDisplay = () => {
        if (multiSelect) return getMultiSelectDisplay();
        if (value === undefined || value === null) return placeholder;

        // If using object options, find the matching option
        if (getOptionValue) {
            const selectedOption = options.find(opt => getValueFromOption(opt) === value);
            return selectedOption ? getDisplayLabel(selectedOption) : placeholder;
        }

        // For string options: if value is empty and not in options, show placeholder
        if (value === '' && !options.includes('')) {
            return placeholder;
        }

        return getDisplayLabel(value);
    };

    const handleToggle = () => {
        if (disabled) return;
        const willOpen = !isOpen;
        setIsOpen(willOpen);
        if (willOpen && onOpen) {
            onOpen();
        }
    };

    return (
        <div className={`relative no-drag ${className}`} ref={wrapperRef}>
            <button
                onClick={handleToggle}
                disabled={disabled}
                className={`w-full flex items-center justify-between px-3 py-2 bg-surface border border-border rounded text-sm text-text transition-colors ${
                    disabled
                        ? 'opacity-50 cursor-not-allowed'
                        : 'hover:border-primary focus:outline-none focus:border-primary'
                }`}
            >
                <span className="truncate">
                    {getSelectedOptionDisplay()}
                </span>
                <ChevronDownIcon className="h-4 w-4 text-gray-400 ml-2 shrink-0" />
            </button>

            {isOpen && (
                <div className="absolute z-50 w-full mt-1 bg-surface border border-border rounded shadow-lg max-h-60 flex flex-col">
                    {(searchable || multiSelect) && (
                    <div className="p-2 border-b border-border sticky top-0 bg-surface">
                        {searchable && (
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
                        )}
                        {multiSelect && !searchTerm && (
                            <button
                                className="w-full mt-2 pl-1 pr-3 py-2 text-sm bg-primary/10 text-primary rounded hover:bg-primary/20 transition-colors flex items-center gap-2"
                                onClick={() => {
                                    if (allSelectionState.allSelected) {
                                        onChange([]);
                                    } else if (!allSelectionState.someSelected) {
                                        onChange(['*']);
                                    } else {
                                        onChange([]);
                                    }
                                }}
                            >
                                <input
                                    type="checkbox"
                                    checked={allSelectionState.allSelected}
                                    ref={(el) => { if (el) el.indeterminate = allSelectionState.indeterminate; }}
                                    onChange={() => {}}
                                />
                                <span>{labels.all} ({allNamespaces.length})</span>
                            </button>
                        )}
                        {multiSelect && searchTerm && nonEmptyFilteredOptions.length > 0 && (
                            <button
                                className="w-full mt-2 pl-1 pr-3 py-2 text-sm bg-primary/10 text-primary rounded hover:bg-primary/20 transition-colors flex items-center gap-2"
                                onClick={() => {
                                    const currentValue = Array.isArray(value) ? value : [];
                                    const { matchingNamespaces, allSelected, someSelected } = matchingSelectionState;

                                    if (allSelected) {
                                        onChange(currentValue.filter(v => !matchingNamespaces.includes(v)));
                                    } else if (!someSelected) {
                                        onChange([...new Set([...currentValue, ...matchingNamespaces])]);
                                    } else {
                                        onChange(currentValue.filter(v => !matchingNamespaces.includes(v)));
                                    }
                                }}
                            >
                                <input
                                    type="checkbox"
                                    checked={matchingSelectionState.allSelected}
                                    ref={(el) => { if (el) el.indeterminate = matchingSelectionState.indeterminate; }}
                                    onChange={() => {}}
                                />
                                <span>Select all matching ({nonEmptyFilteredOptions.length})</span>
                            </button>
                        )}
                    </div>
                    )}
                    <div className="overflow-y-auto flex-1">
                        {nonEmptyFilteredOptions.length > 0 ? (
                            nonEmptyFilteredOptions.map((option) => (
                                <div
                                    key={getValueFromOption(option) || '__all__'}
                                    className={`px-3 py-2 text-sm cursor-pointer hover:bg-primary/10 flex items-center gap-2 ${isOptionSelected(option) ? 'text-primary font-medium' : 'text-text'}`}
                                    onClick={() => handleOptionClick(option)}
                                >
                                    {multiSelect && (
                                        <input
                                            type="checkbox"
                                            checked={isOptionSelected(option)}
                                            onChange={() => {}}
                                        />
                                    )}
                                    {renderOption ? (
                                        renderOption(option, isOptionSelected(option))
                                    ) : (
                                        <span className="flex-1">{getDisplayLabel(option)}</span>
                                    )}
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
