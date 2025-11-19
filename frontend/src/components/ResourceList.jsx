import React, { useState } from 'react';

export default function ResourceList({ title, columns, data, isLoading }) {
    const [searchTerm, setSearchTerm] = useState('');

    const filteredData = data.filter(item => {
        if (!searchTerm) return true;
        // Assuming 'name' is usually in metadata.name, but we can check columns too or just check metadata.name safely
        const name = item.metadata?.name || '';
        return name.toLowerCase().includes(searchTerm.toLowerCase());
    });

    return (
        <div className="flex flex-col h-full bg-background">
            <div className="px-4 py-3 border-b border-border flex justify-between items-center bg-surface">
                <h2 className="text-lg font-semibold text-text">{title}</h2>
                <input
                    type="text"
                    placeholder="Search..."
                    value={searchTerm}
                    onChange={(e) => setSearchTerm(e.target.value)}
                    className="bg-background border border-border text-text text-sm rounded px-3 py-1 focus:outline-none focus:border-primary w-64"
                />
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
                                <tr key={item.metadata?.uid || index} className="hover:bg-surface transition-colors">
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
