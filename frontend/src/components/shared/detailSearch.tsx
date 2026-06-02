import React, { useState } from 'react';
import { MagnifyingGlassIcon, XMarkIcon } from '@heroicons/react/24/outline';

export const normalizeSearchTerm = (term: string) => term.trim().toLowerCase();

export const matchesSearch = (parts: any[], term: string) => {
    const normalizedTerm = normalizeSearchTerm(term);
    if (!normalizedTerm) return true;

    return parts.some((part) => {
        if (part === null || part === undefined) return false;
        return String(part).toLowerCase().includes(normalizedTerm);
    });
};

export const entriesFromObject = (obj: any) => Object.entries(obj || {})
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([key, value]) => ({
        key,
        value,
        display: `${key}=${value}`
    }));

export const SectionSearchInput = ({
    value,
    onChange,
    placeholder
}: { value: string; onChange: (value: string) => void; placeholder: string }) => (
    <div className="relative w-44 max-w-full">
        <MagnifyingGlassIcon className="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-gray-500 pointer-events-none" />
        <input
            type="text"
            value={value}
            onChange={(e: any) => onChange(e.target.value)}
            placeholder={placeholder}
            className="w-full pl-7 pr-7 py-1 text-xs bg-surface border border-border rounded text-gray-200 placeholder:text-gray-500 focus:outline-none focus:border-primary transition-colors"
            autoComplete="off"
            autoCorrect="off"
            autoCapitalize="off"
            spellCheck="false"
        />
        {value && (
            <button
                type="button"
                onClick={() => onChange('')}
                className="absolute right-1.5 top-1/2 -translate-y-1/2 p-0.5 text-gray-500 hover:text-gray-300 rounded"
                title="Clear search"
            >
                <XMarkIcon className="w-3.5 h-3.5" />
            </button>
        )}
    </div>
);

export const NoSectionMatches = ({ term }: { term: string }) => (
    <div className="text-gray-500 text-sm text-center py-4">
        No matches for "{term}"
    </div>
);

export const useSectionSearch = () => {
    const [sectionSearch, setSectionSearch] = useState<Record<string, string>>({});

    const getSectionTerm = (sectionKey: string) => sectionSearch[sectionKey] || '';
    const setSectionTerm = (sectionKey: string, value: string) => {
        setSectionSearch((prev) => ({
            ...prev,
            [sectionKey]: value
        }));
    };
    const renderSearch = (sectionKey: string, placeholder: string) => (
        <SectionSearchInput
            value={getSectionTerm(sectionKey)}
            onChange={(value) => setSectionTerm(sectionKey, value)}
            placeholder={placeholder}
        />
    );

    return {
        sectionSearch,
        getSectionTerm,
        setSectionTerm,
        renderSearch,
    };
};
