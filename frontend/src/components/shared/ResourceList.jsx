import React, { useState, useRef, useEffect, useMemo } from 'react';
import { MagnifyingGlassIcon, EllipsisVerticalIcon, InformationCircleIcon } from '@heroicons/react/24/outline';
import SearchSelect from './SearchSelect';
import { createFilter, getFieldsMetadata } from '../../utils/search';

export default function ResourceList({
    title,
    columns,
    data,
    isLoading,
    namespaces = [],
    currentNamespace,
    onNamespaceChange,
    showNamespaceSelector = true,
    multiSelectNamespaces = false,
    highlightedUid = null,
    initialSort = null,
    resourceType = null
}) {
    const [sortConfig, setSortConfig] = useState(initialSort || { key: null, direction: 'asc' });
    const [searchTerm, setSearchTerm] = useState('');
    const [hiddenColumns, setHiddenColumns] = useState(new Set());
    const [showColumnMenu, setShowColumnMenu] = useState(false);
    const [showSearchHelp, setShowSearchHelp] = useState(false);
    const columnMenuRef = useRef(null);
    const searchHelpRef = useRef(null);

    useEffect(() => {
        const handleClickOutside = (event) => {
            if (columnMenuRef.current && !columnMenuRef.current.contains(event.target)) {
                setShowColumnMenu(false);
            }
            if (searchHelpRef.current && !searchHelpRef.current.contains(event.target)) {
                setShowSearchHelp(false);
            }
        };

        document.addEventListener('mousedown', handleClickOutside);
        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, []);

    const toggleColumn = (key) => {
        const newHidden = new Set(hiddenColumns);
        if (newHidden.has(key)) {
            newHidden.delete(key);
        } else {
            newHidden.add(key);
        }
        setHiddenColumns(newHidden);
    };

    const visibleColumns = columns.filter(col => !hiddenColumns.has(col.key));

    const handleSort = (key) => {
        let direction = 'asc';
        if (sortConfig.key === key && sortConfig.direction === 'asc') {
            direction = 'desc';
        }
        setSortConfig({ key, direction });
    };

    const filteredData = useMemo(() => {
        if (!searchTerm) return data;
        const filterFn = createFilter(resourceType, searchTerm);
        return data.filter(filterFn);
    }, [data, searchTerm, resourceType]);

    const sortedData = React.useMemo(() => {
        if (!sortConfig.key) return filteredData;

        return [...filteredData].sort((a, b) => {
            const column = columns.find(col => col.key === sortConfig.key);
            if (!column) return 0;

            const getValue = column.getValue || ((item) => item[column.key]);
            const aValue = getValue(a);
            const bValue = getValue(b);

            if (aValue < bValue) {
                return sortConfig.direction === 'asc' ? -1 : 1;
            }
            if (aValue > bValue) {
                return sortConfig.direction === 'asc' ? 1 : -1;
            }

            // Tie-breakers (always ascending)

            // 1. Age (if available and not primary sort)
            if (sortConfig.key !== 'age') {
                const aDate = a.metadata?.creationTimestamp;
                const bDate = b.metadata?.creationTimestamp;
                if (aDate && bDate && aDate !== bDate) {
                    // User wants "Ascending Age" (Newest to Oldest)
                    // Newest = Larger Timestamp
                    return aDate > bDate ? -1 : 1;
                }
            }

            // 2. Name (if not primary sort)
            if (sortConfig.key !== 'name') {
                const aName = a.metadata?.name || '';
                const bName = b.metadata?.name || '';
                return aName.localeCompare(bName);
            }

            return 0;
        });
    }, [filteredData, sortConfig, columns]);

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header */}
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0 gap-4">
                <div className="flex items-center gap-4 flex-1">
                    <h1 className="text-lg font-semibold text-text shrink-0">{title}</h1>

                    {/* Search Bar */}
                    <div className="relative max-w-md w-full flex items-center gap-1">
                        <div className="relative flex-1">
                            <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-3 top-1/2 transform -translate-y-1/2" />
                            <input
                                type="text"
                                placeholder={resourceType ? `Search... (name:"x" status:Running)` : `Search ${title}...`}
                                className="w-full bg-background border border-border rounded-md pl-9 pr-4 py-1.5 text-sm text-text focus:outline-none focus:border-primary transition-colors"
                                value={searchTerm}
                                onChange={(e) => setSearchTerm(e.target.value)}
                                autoComplete="off"
                                autoCorrect="off"
                                spellCheck="false"
                            />
                        </div>
                        {resourceType && (
                            <div className="relative" ref={searchHelpRef}>
                                <button
                                    onClick={() => setShowSearchHelp(!showSearchHelp)}
                                    className="p-1 text-gray-400 hover:text-gray-300 transition-colors"
                                    title="Search syntax help"
                                >
                                    <InformationCircleIcon className="h-5 w-5" />
                                </button>
                                {showSearchHelp && (
                                    <div className="absolute left-0 top-full mt-1 w-80 bg-surface border border-border rounded-md shadow-lg z-50 p-3 text-sm">
                                        <div className="font-medium text-text mb-2">Search Syntax</div>
                                        <div className="space-y-2 text-gray-400">
                                            <div>
                                                <span className="text-gray-300">Plain text:</span> matches name
                                                <div className="text-xs text-gray-500 ml-2">nginx</div>
                                            </div>
                                            <div>
                                                <span className="text-gray-300">Field search:</span> field:"value"
                                                <div className="text-xs text-gray-500 ml-2">name:"my-pod" status:Running</div>
                                            </div>
                                            <div>
                                                <span className="text-gray-300">Regex:</span> field:/pattern/
                                                <div className="text-xs text-gray-500 ml-2">name:/^nginx-/ name:/end$/</div>
                                            </div>
                                            <div className="border-t border-border pt-2 mt-2">
                                                <span className="text-gray-300">Available fields:</span>
                                                <div className="text-xs text-gray-500 mt-1 flex flex-wrap gap-1">
                                                    {getFieldsMetadata(resourceType).map(f => (
                                                        <span key={f.name} className="bg-background px-1.5 py-0.5 rounded">
                                                            {f.name}
                                                        </span>
                                                    ))}
                                                </div>
                                            </div>
                                        </div>
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                </div>

                {/* Namespace Selector */}
                {showNamespaceSelector && (
                    <div className="w-64">
                        <SearchSelect
                            options={namespaces}
                            value={currentNamespace}
                            onChange={onNamespaceChange}
                            placeholder="Select Namespace..."
                            multiSelect={multiSelectNamespaces}
                        />
                    </div>
                )}
            </div>
            <div className="flex-1 overflow-auto">
                <table className="w-full text-left border-collapse">
                    <thead className="bg-surface sticky top-0 z-10">
                        <tr>
                            {visibleColumns.map((col) => (
                                <th
                                    key={col.key}
                                    className={`p-3 text-xs font-medium text-gray-400 uppercase tracking-wider border-b border-border select-none ${col.isColumnSelector ? '' : 'cursor-pointer hover:text-text'}`}
                                    onClick={() => !col.isColumnSelector && handleSort(col.key)}
                                >
                                    {col.isColumnSelector ? (
                                        <div className={`relative flex ${col.align === 'center' ? 'justify-center' : 'justify-end'}`} ref={columnMenuRef}>
                                            <button
                                                onClick={(e) => {
                                                    e.stopPropagation();
                                                    setShowColumnMenu(!showColumnMenu);
                                                }}
                                                className="p-1 hover:bg-white/10 rounded-full transition-colors"
                                            >
                                                <EllipsisVerticalIcon className="h-5 w-5" />
                                            </button>
                                            {showColumnMenu && (
                                                <div className="absolute right-0 top-full mt-1 w-48 bg-surface border border-border rounded-md shadow-lg z-50 py-1">
                                                    {columns.filter(c => !c.isColumnSelector).map(c => (
                                                        <label
                                                            key={c.key}
                                                            className="flex items-center px-4 py-2 text-sm text-text hover:bg-white/5 cursor-pointer"
                                                            onClick={(e) => e.stopPropagation()}
                                                        >
                                                            <input
                                                                type="checkbox"
                                                                checked={!hiddenColumns.has(c.key)}
                                                                onChange={() => toggleColumn(c.key)}
                                                                className="mr-2 rounded border-gray-600 bg-background text-primary focus:ring-primary"
                                                            />
                                                            {c.label}
                                                        </label>
                                                    ))}
                                                </div>
                                            )}
                                        </div>
                                    ) : (
                                        <div className={`flex items-center gap-1 ${col.align === 'center' ? 'justify-center' : ''}`}>
                                            {col.label}
                                            {sortConfig.key === col.key && (
                                                <span className="text-primary">
                                                    {sortConfig.direction === 'asc' ? '↑' : '↓'}
                                                </span>
                                            )}
                                        </div>
                                    )}
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                        {isLoading ? (
                            <tr>
                                <td colSpan={visibleColumns.length} className="p-4 text-center text-gray-500">
                                    Loading...
                                </td>
                            </tr>
                        ) : sortedData.length === 0 ? (
                            <tr>
                                <td colSpan={visibleColumns.length} className="p-4 text-center text-gray-500">
                                    No resources found
                                </td>
                            </tr>
                        ) : (
                            sortedData.map((item, index) => (
                                <tr
                                    key={item.metadata?.uid || index}
                                    className={`transition-colors ${highlightedUid === item.metadata?.uid ? 'bg-white/5' : 'hover:bg-white/5'}`}
                                >
                                    {visibleColumns.map((col) => {
                                        const content = col.render ? col.render(item) : item[col.key];
                                        const isNamespaceColumn = col.key === 'namespace' && onNamespaceChange;
                                        const namespaceValue = item.metadata?.namespace;

                                        return (
                                            <td key={col.key} className={`p-3 text-sm text-text whitespace-nowrap ${col.align === 'center' ? 'text-center' : ''}`}>
                                                {isNamespaceColumn && namespaceValue ? (
                                                    <button
                                                        onClick={(e) => {
                                                            e.stopPropagation();
                                                            onNamespaceChange([namespaceValue]);
                                                        }}
                                                        className="text-primary hover:text-primary/80 hover:underline transition-colors"
                                                        title={`Filter to namespace: ${namespaceValue}`}
                                                    >
                                                        {content}
                                                    </button>
                                                ) : content}
                                            </td>
                                        );
                                    })}
                                </tr>
                            ))
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
