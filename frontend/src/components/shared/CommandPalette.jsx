import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { MagnifyingGlassIcon } from '@heroicons/react/24/outline';
import { useCommandPaletteItems } from '../../hooks/useCommandPaletteItems';
import { useUI } from '../../context/UIContext';

/**
 * CommandPalette - VS Code-style command palette for quick navigation
 *
 * Triggered by Cmd+Shift+P (Mac) / Ctrl+Shift+P (Windows)
 * Allows searching and navigating to any resource view including CRDs
 */
export default function CommandPalette({ isOpen, onClose }) {
    const { setActiveView } = useUI();
    const { items, loading } = useCommandPaletteItems();
    const [query, setQuery] = useState('');
    const [selectedIndex, setSelectedIndex] = useState(0);
    const inputRef = useRef(null);
    const listRef = useRef(null);

    // Filter items based on query
    const filteredItems = useMemo(() => {
        if (!query.trim()) {
            return items;
        }

        const lowerQuery = query.toLowerCase();

        // Filter and score items
        const scored = items
            .map(item => {
                const labelLower = item.label.toLowerCase();
                const pathLower = item.path.toLowerCase();

                let score = 0;

                // Exact label match - highest priority
                if (labelLower === lowerQuery) {
                    score = 100;
                }
                // Label starts with query
                else if (labelLower.startsWith(lowerQuery)) {
                    score = 80;
                }
                // Label contains query
                else if (labelLower.includes(lowerQuery)) {
                    score = 60;
                }
                // Path contains query (e.g., searching for group name)
                else if (pathLower.includes(lowerQuery)) {
                    score = 40;
                }

                return { item, score };
            })
            .filter(({ score }) => score > 0)
            .sort((a, b) => b.score - a.score);

        return scored.map(({ item }) => item);
    }, [items, query]);

    // Reset state when opening
    useEffect(() => {
        if (isOpen) {
            setQuery('');
            setSelectedIndex(0);
            // Focus input after a short delay to ensure it's mounted
            setTimeout(() => {
                inputRef.current?.focus();
            }, 10);
        }
    }, [isOpen]);

    // Keep selected index in bounds
    useEffect(() => {
        if (selectedIndex >= filteredItems.length) {
            setSelectedIndex(Math.max(0, filteredItems.length - 1));
        }
    }, [filteredItems.length, selectedIndex]);

    // Scroll selected item into view
    useEffect(() => {
        if (listRef.current && filteredItems.length > 0) {
            const selectedElement = listRef.current.children[selectedIndex];
            if (selectedElement) {
                selectedElement.scrollIntoView({ block: 'nearest' });
            }
        }
    }, [selectedIndex, filteredItems.length]);

    // Handle selection
    const handleSelect = useCallback((item) => {
        setActiveView(item.viewId);
        onClose();
    }, [setActiveView, onClose]);

    // Handle keyboard navigation
    const handleKeyDown = useCallback((e) => {
        switch (e.key) {
            case 'ArrowDown':
                e.preventDefault();
                setSelectedIndex(prev =>
                    prev < filteredItems.length - 1 ? prev + 1 : prev
                );
                break;
            case 'ArrowUp':
                e.preventDefault();
                setSelectedIndex(prev => prev > 0 ? prev - 1 : 0);
                break;
            case 'Enter':
                e.preventDefault();
                if (filteredItems[selectedIndex]) {
                    handleSelect(filteredItems[selectedIndex]);
                }
                break;
            case 'Escape':
                e.preventDefault();
                onClose();
                break;
            default:
                break;
        }
    }, [filteredItems, selectedIndex, handleSelect, onClose]);

    // Handle backdrop click
    const handleBackdropClick = useCallback((e) => {
        if (e.target === e.currentTarget) {
            onClose();
        }
    }, [onClose]);

    if (!isOpen) return null;

    return createPortal(
        <div
            className="fixed inset-0 bg-black/60 flex items-start justify-center pt-[15vh] z-50"
            onClick={handleBackdropClick}
        >
            <div
                className="bg-surface border border-border rounded-lg shadow-2xl w-full max-w-xl flex flex-col overflow-hidden"
                onClick={(e) => e.stopPropagation()}
            >
                {/* Search Input */}
                <div className="flex items-center gap-3 px-4 py-3 border-b border-border">
                    <MagnifyingGlassIcon className="h-5 w-5 text-gray-400 shrink-0" />
                    <input
                        ref={inputRef}
                        type="text"
                        value={query}
                        onChange={(e) => {
                            setQuery(e.target.value);
                            setSelectedIndex(0);
                        }}
                        onKeyDown={handleKeyDown}
                        placeholder="Search resources..."
                        className="flex-1 bg-transparent text-text placeholder-gray-500 outline-none text-sm"
                        autoComplete="off"
                        spellCheck={false}
                    />
                    {loading && (
                        <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary shrink-0" />
                    )}
                </div>

                {/* Results List */}
                <div
                    ref={listRef}
                    className="max-h-[50vh] overflow-y-auto"
                >
                    {filteredItems.length === 0 ? (
                        <div className="px-4 py-8 text-center text-gray-500 text-sm">
                            {query ? 'No results found' : 'No resources available'}
                        </div>
                    ) : (
                        filteredItems.map((item, index) => (
                            <button
                                key={item.id}
                                onClick={() => handleSelect(item)}
                                onMouseEnter={() => setSelectedIndex(index)}
                                className={`w-full px-4 py-2.5 flex items-center text-left transition-colors ${
                                    index === selectedIndex
                                        ? 'bg-primary/20 text-text'
                                        : 'text-gray-300 hover:bg-white/5'
                                }`}
                            >
                                <span className="text-sm truncate">
                                    {/* Render path with dimmed group names */}
                                    {item.path.split(' > ').map((part, i, arr) => (
                                        <span key={i}>
                                            {i < arr.length - 1 ? (
                                                <>
                                                    <span className="text-gray-500">{part}</span>
                                                    <span className="text-gray-600 mx-1.5">&gt;</span>
                                                </>
                                            ) : (
                                                <span className={index === selectedIndex ? 'text-text font-medium' : 'text-gray-200'}>
                                                    {part}
                                                </span>
                                            )}
                                        </span>
                                    ))}
                                </span>
                            </button>
                        ))
                    )}
                </div>

                {/* Footer with keyboard hints */}
                <div className="px-4 py-2 border-t border-border flex items-center gap-4 text-xs text-gray-500">
                    <span><kbd className="px-1.5 py-0.5 bg-background rounded border border-border">↑↓</kbd> navigate</span>
                    <span><kbd className="px-1.5 py-0.5 bg-background rounded border border-border">↵</kbd> select</span>
                    <span><kbd className="px-1.5 py-0.5 bg-background rounded border border-border">esc</kbd> close</span>
                </div>
            </div>
        </div>,
        document.body
    );
}
