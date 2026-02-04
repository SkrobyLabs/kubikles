import React, { useState, useEffect, useCallback, useRef } from 'react';
import { useConfig } from '../../../context';
import { useUI } from '../../../context';
import { XMarkIcon, ArrowPathIcon, MagnifyingGlassIcon } from '@heroicons/react/24/outline';
import ConfigSidebar from './ConfigSidebar';
import ConfigSection from './ConfigSection';
import { getSortedSections, searchFields, getModifiedFields } from '../../../config/configSchema';

export default function ConfigEditorUI({ onSwitchMode }) {
    const { config, setConfig, resetConfig, closeConfigEditor } = useConfig();
    const { openModal, closeModal } = useUI();
    const [activeSection, setActiveSection] = useState(() => getSortedSections()[0]);
    const [saved, setSaved] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const [showSearch, setShowSearch] = useState(false);
    const searchInputRef = useRef(null);

    // Search results - which sections/fields match
    const searchResults = searchTerm ? searchFields(searchTerm) : null;

    // Auto-save on field change
    const handleFieldChange = useCallback((path, value) => {
        setConfig(path, value);
        setSaved(true);
    }, [setConfig]);

    // Clear saved indicator after brief delay
    useEffect(() => {
        if (saved) {
            const timer = setTimeout(() => setSaved(false), 1500);
            return () => clearTimeout(timer);
        }
    }, [saved]);

    // Cmd+F to open search
    useEffect(() => {
        const handleKeyDown = (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'f') {
                e.preventDefault();
                setShowSearch(true);
                setTimeout(() => searchInputRef.current?.focus(), 0);
            }
            if (e.key === 'Escape' && showSearch) {
                setShowSearch(false);
                setSearchTerm('');
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [showSearch]);

    // Auto-select first matching section when search changes
    useEffect(() => {
        if (searchResults) {
            const matchingSections = Object.keys(searchResults);
            if (matchingSections.length > 0 && !matchingSections.includes(activeSection)) {
                setActiveSection(matchingSections[0]);
            }
        }
    }, [searchResults, activeSection]);

    const handleReset = () => {
        const modifiedFields = getModifiedFields(config);

        if (modifiedFields.length === 0) {
            openModal({
                title: 'No Changes',
                content: 'All settings are already at their default values.',
                confirmText: 'OK',
                confirmStyle: 'primary',
                onConfirm: closeModal
            });
            return;
        }

        const formatValue = (val) => {
            if (typeof val === 'boolean') return val ? 'Yes' : 'No';
            return String(val);
        };

        const content = (
            <div className="space-y-3">
                <p>The following settings will be reset to defaults:</p>
                <ul className={`space-y-1 text-sm ${modifiedFields.length > 10 ? 'max-h-64 overflow-y-auto pr-2' : ''}`}>
                    {modifiedFields.map(({ path, label, currentValue, defaultValue }) => (
                        <li key={path} className="flex items-center gap-2">
                            <span className="text-text">{label}:</span>
                            <span className="text-red-400 line-through">{formatValue(currentValue)}</span>
                            <span className="text-gray-500">→</span>
                            <span className="text-green-400">{formatValue(defaultValue)}</span>
                        </li>
                    ))}
                </ul>
            </div>
        );

        openModal({
            title: 'Reset to Defaults',
            content,
            confirmText: 'Reset',
            confirmStyle: 'danger',
            onConfirm: () => {
                resetConfig();
                closeModal();
            }
        });
    };

    const clearSearch = () => {
        setSearchTerm('');
        setShowSearch(false);
    };

    return (
        <div className="h-full flex flex-col bg-background">
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0 titlebar-drag">
                <h2 className="text-sm font-semibold text-text">Settings</h2>
                <div className="flex items-center gap-3">
                    {/* Save feedback */}
                    {saved && (
                        <span className="text-sm text-green-400">Saved</span>
                    )}

                    {/* Search */}
                    {showSearch ? (
                        <div className="relative">
                            <MagnifyingGlassIcon className="w-4 h-4 text-gray-400 absolute left-2 top-1/2 -translate-y-1/2" />
                            <input
                                ref={searchInputRef}
                                type="text"
                                value={searchTerm}
                                onChange={(e) => setSearchTerm(e.target.value)}
                                placeholder="Search settings..."
                                className="w-48 pl-8 pr-8 py-1 text-sm bg-background border border-border rounded text-text focus:outline-none focus:border-primary"
                                autoComplete="off"
                                autoCorrect="off"
                                autoCapitalize="off"
                                spellCheck="false"
                                data-form-type="other"
                            />
                            {searchTerm && (
                                <button
                                    onClick={clearSearch}
                                    className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-400 hover:text-white"
                                >
                                    <XMarkIcon className="w-4 h-4" />
                                </button>
                            )}
                        </div>
                    ) : (
                        <button
                            onClick={() => {
                                setShowSearch(true);
                                setTimeout(() => searchInputRef.current?.focus(), 0);
                            }}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                            title="Search (⌘F)"
                        >
                            <MagnifyingGlassIcon className="w-4 h-4" />
                        </button>
                    )}

                    {/* Mode Toggle */}
                    <div className="flex items-center bg-background rounded overflow-hidden text-xs">
                        <button
                            className="px-3 py-1.5 bg-primary text-white"
                        >
                            UI
                        </button>
                        <button
                            onClick={() => onSwitchMode('flat')}
                            className="px-3 py-1.5 text-gray-400 hover:text-white transition-colors"
                        >
                            Flat
                        </button>
                        <button
                            onClick={() => onSwitchMode('json')}
                            className="px-3 py-1.5 text-gray-400 hover:text-white transition-colors"
                        >
                            JSON
                        </button>
                    </div>

                    <div className="w-px h-5 bg-border" />

                    {/* Actions */}
                    <button
                        onClick={handleReset}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                    >
                        <ArrowPathIcon className="w-4 h-4" />
                        Reset
                    </button>
                    <button
                        onClick={closeConfigEditor}
                        className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                    >
                        <XMarkIcon className="w-5 h-5" />
                    </button>
                </div>
            </div>

            {/* Content: Sidebar + Section */}
            <div className="flex-1 flex overflow-hidden">
                <ConfigSidebar
                    activeSection={activeSection}
                    onSectionChange={setActiveSection}
                    searchResults={searchResults}
                />
                <div className="flex-1 overflow-y-auto p-6">
                    <ConfigSection
                        section={activeSection}
                        config={config}
                        onFieldChange={handleFieldChange}
                        searchResults={searchResults}
                    />
                </div>
            </div>
        </div>
    );
}
