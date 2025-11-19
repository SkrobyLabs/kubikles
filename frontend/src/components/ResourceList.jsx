import React, { useState } from 'react';
import { MagnifyingGlassIcon } from '@heroicons/react/24/outline';
import SearchSelect from './SearchSelect';

export default function ResourceList({
    title,
    columns,
    data,
    isLoading,
    namespaces = [],
    currentNamespace,
    onNamespaceChange,
    showNamespaceSelector = true,
    highlightedUid = null
}) {
    const [searchTerm, setSearchTerm] = useState('');

    const filteredData = data.filter(item =>
        item.metadata?.name?.toLowerCase().includes(searchTerm.toLowerCase())
    );

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header */}
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0 gap-4">
                <div className="flex items-center gap-4 flex-1">
                    <h1 className="text-lg font-semibold text-text shrink-0">{title}</h1>

                    {/* Search Bar */}
                    <div className="relative max-w-md w-full">
                        <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-3 top-1/2 transform -translate-y-1/2" />
                        <input
                            type="text"
                            placeholder={`Search ${title}...`}
                            className="w-full bg-background border border-border rounded-md pl-9 pr-4 py-1.5 text-sm text-text focus:outline-none focus:border-primary transition-colors"
                            value={searchTerm}
                            onChange={(e) => setSearchTerm(e.target.value)}
                        />
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
                        />
                    </div>
                )}
            </div>
            <div className="flex-1 overflow-auto">
                <table className="w-full text-left border-collapse">
                    <thead className="bg-surface sticky top-0 z-10">
                        <tr>
                            {columns.map((col) => (
                                <th key={col.key} className="p-3 text-xs font-medium text-gray-400 uppercase tracking-wider border-b border-border">
                                    {col.label}
                                </th>
                            ))}
                        </tr>
                    </thead>
                    <tbody className="divide-y divide-border">
                        {isLoading ? (
                            <tr>
                                <td colSpan={columns.length} className="p-4 text-center text-gray-500">
                                    Loading...
                                </td>
                            </tr>
                        ) : filteredData.length === 0 ? (
                            <tr>
                                <td colSpan={columns.length} className="p-4 text-center text-gray-500">
                                    No resources found
                                </td>
                            </tr>
                        ) : (
                            filteredData.map((item, index) => (
                                <tr
                                    key={item.metadata?.uid || index}
                                    className={`transition-colors ${highlightedUid === item.metadata?.uid ? 'bg-white/5' : 'hover:bg-white/5'}`}
                                >
                                    {columns.map((col) => (
                                        <td key={col.key} className="p-3 text-sm text-text whitespace-nowrap">
                                            {col.render ? col.render(item) : item[col.key]}
                                        </td>
                                    ))}
                                </tr>
                            ))
                        )}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
