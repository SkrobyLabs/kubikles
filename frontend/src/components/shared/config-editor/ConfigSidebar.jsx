import React from 'react';
import { configSchema, getSortedSections } from '../../../config/configSchema';

export default function ConfigSidebar({ activeSection, onSectionChange, searchResults }) {
    const allSections = getSortedSections();

    // Filter sections if search is active
    const sections = searchResults
        ? allSections.filter(s => searchResults[s])
        : allSections;

    if (sections.length === 0) {
        return (
            <nav className="w-48 shrink-0 border-r border-border p-3">
                <p className="text-sm text-text-muted px-3 py-2">No matches</p>
            </nav>
        );
    }

    return (
        <nav className="w-48 shrink-0 border-r border-border p-3 space-y-1">
            {sections.map((section) => {
                const meta = configSchema[section]?._meta;
                const isActive = section === activeSection;
                const matchCount = searchResults?.[section]?.length;

                return (
                    <button
                        key={section}
                        onClick={() => onSectionChange(section)}
                        className={`w-full text-left px-3 py-2 rounded text-sm transition-colors ${
                            isActive
                                ? 'bg-primary/10 text-primary font-medium'
                                : 'text-text-muted hover:text-text hover:bg-surface'
                        }`}
                    >
                        <span>{meta?.label || section}</span>
                        {matchCount && matchCount !== '*' && (
                            <span className="ml-2 text-xs text-primary">({matchCount})</span>
                        )}
                    </button>
                );
            })}
        </nav>
    );
}
