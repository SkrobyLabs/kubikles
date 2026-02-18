import React, { useState, useRef, useEffect, useCallback } from 'react';
import { BookmarkIcon, TrashIcon, PencilIcon, ChevronDownIcon, PlusIcon, DocumentDuplicateIcon, StarIcon, ArrowDownTrayIcon, CheckIcon } from '@heroicons/react/24/outline';
import { BookmarkIcon as BookmarkSolidIcon, StarIcon as StarSolidIcon } from '@heroicons/react/24/solid';

/**
 * Saved Views Dropdown
 *
 * Dropdown to save, load, and manage filter views.
 */
export default function SavedViewsDropdown({
    views = [],
    activeViewId = null,
    isDirty = false,
    onSave,
    onLoad,
    onUpdate,
    onDelete,
    onRename,
    onDuplicate,
    onSetDefault,
    getCurrentConfig,
}: any) {
    const [isOpen, setIsOpen] = useState(false);
    const [isSaving, setIsSaving] = useState(false);
    const [newViewName, setNewViewName] = useState('');
    const [editingId, setEditingId] = useState<any>(null);
    const [editName, setEditName] = useState('');
    const [savedViewId, setSavedViewId] = useState<any>(null); // For save feedback
    const dropdownRef = useRef<any>(null);
    const inputRef = useRef<any>(null);

    // Close dropdown when clicking outside
    useEffect(() => {
        const handleClickOutside = (e: any) => {
            if (dropdownRef.current && !dropdownRef.current.contains(e.target)) {
                setIsOpen(false);
                setIsSaving(false);
                setEditingId(null);
            }
        };
        if (isOpen) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [isOpen]);

    // Focus input when saving
    useEffect(() => {
        if (isSaving && inputRef.current) {
            inputRef.current.focus();
        }
    }, [isSaving]);

    const handleSave = useCallback(() => {
        if (!newViewName.trim()) return;

        const config = getCurrentConfig();
        onSave(newViewName.trim(), config);
        setNewViewName('');
        setIsSaving(false);
    }, [newViewName, getCurrentConfig, onSave]);

    const handleSaveKeyDown = useCallback((e: any) => {
        if (e.key === 'Enter') {
            handleSave();
        } else if (e.key === 'Escape') {
            setIsSaving(false);
            setNewViewName('');
        }
    }, [handleSave]);

    const handleRename = useCallback((viewId: any) => {
        if (!editName.trim()) return;
        onRename(viewId, editName.trim());
        setEditingId(null);
        setEditName('');
    }, [editName, onRename]);

    const handleRenameKeyDown = useCallback((e: any, viewId: any) => {
        if (e.key === 'Enter') {
            handleRename(viewId);
        } else if (e.key === 'Escape') {
            setEditingId(null);
            setEditName('');
        }
    }, [handleRename]);

    const handleLoad = useCallback((viewId: any) => {
        // If clicking the already-active view, don't reload or close — let user access edit actions
        if (viewId === activeViewId) return;
        onLoad(viewId);
        setIsOpen(false);
    }, [onLoad, activeViewId]);

    const handleDelete = useCallback((e: any, viewId: any) => {
        e.stopPropagation();
        onDelete(viewId);
    }, [onDelete]);

    const handleStartEdit = useCallback((e: any, view: any) => {
        e.stopPropagation();
        setEditingId(view.id);
        setEditName(view.name);
    }, []);

    const handleDuplicate = useCallback((e: any, viewId: any) => {
        e.stopPropagation();
        onDuplicate?.(viewId);
    }, [onDuplicate]);

    const handleSetDefault = useCallback((e: any, viewId: any) => {
        e.stopPropagation();
        onSetDefault?.(viewId);
    }, [onSetDefault]);

    const handleUpdate = useCallback((viewId: any) => {
        if (!viewId || !onUpdate) return;
        const config = getCurrentConfig();
        onUpdate(viewId, config);
        // Show success feedback
        setSavedViewId(viewId);
        setTimeout(() => setSavedViewId(null), 1500);
    }, [onUpdate, getCurrentConfig]);

    const activeView = activeViewId ? views.find((v: any) => v.id === activeViewId) : null;
    const hasActiveView = !!activeView;

    return (
        <div className="relative no-drag" ref={dropdownRef}>
            <button
                onClick={() => setIsOpen(!isOpen)}
                className={`flex items-center gap-1.5 px-3 py-1.5 text-sm border rounded-md transition-colors ${
                    isDirty
                        ? 'text-amber-400 border-amber-400/50 bg-amber-400/10 hover:bg-amber-400/20'
                        : hasActiveView
                            ? 'text-primary border-primary/50 bg-primary/10 hover:bg-primary/20'
                            : 'text-gray-400 border-border hover:text-white hover:bg-white/5'
                }`}
                title={hasActiveView
                    ? (isDirty ? `Active view: ${activeView.name} (unsaved changes)` : `Active view: ${activeView.name}`)
                    : (isDirty ? 'Unsaved changes — save as a view' : 'Saved Views')
                }
            >
                {hasActiveView ? (
                    <BookmarkSolidIcon className={`w-4 h-4 ${isDirty ? 'text-amber-400' : ''}`} />
                ) : (
                    <BookmarkIcon className={`w-4 h-4 ${isDirty ? 'text-amber-400' : ''}`} />
                )}
                <span className="hidden sm:inline truncate max-w-[120px]">
                    {hasActiveView ? activeView.name : 'Views'}
                </span>
                <ChevronDownIcon className={`w-3 h-3 transition-transform shrink-0 ${isOpen ? 'rotate-180' : ''}`} />
            </button>

            {isOpen && (
                <div className="absolute right-0 top-full mt-1 bg-surface border border-border rounded-lg shadow-xl z-50 w-72">
                    {/* Header */}
                    <div className="px-3 py-2 border-b border-border flex items-center justify-between">
                        <span className="text-xs font-medium text-gray-400 uppercase tracking-wider">Saved Views</span>
                        <button
                            onClick={() => setIsSaving(true)}
                            className="flex items-center gap-1 text-xs text-primary hover:text-white transition-colors"
                        >
                            <PlusIcon className="w-3.5 h-3.5" />
                            Save current
                        </button>
                    </div>

                    {/* Save new view input */}
                    {isSaving && (
                        <div className="px-3 py-2 border-b border-border">
                            <div className="flex items-center gap-2">
                                <input
                                    ref={inputRef}
                                    type="text"
                                    value={newViewName}
                                    onChange={(e: any) => setNewViewName(e.target.value)}
                                    onKeyDown={handleSaveKeyDown}
                                    placeholder="View name..."
                                    className="flex-1 min-w-0 px-2 py-1 bg-background border border-border rounded text-sm focus:outline-none focus:border-primary"
                                    autoComplete="off"
                                />
                                <button
                                    onClick={handleSave}
                                    disabled={!newViewName.trim()}
                                    className="px-2 py-1 text-xs bg-primary hover:bg-primary/80 text-white rounded transition-colors disabled:opacity-50 shrink-0"
                                >
                                    Save
                                </button>
                            </div>
                        </div>
                    )}

                    {/* View List */}
                    <div className="max-h-60 overflow-auto py-1">
                        {views.length === 0 ? (
                            <div className="px-3 py-4 text-xs text-gray-500 text-center">
                                No saved views yet.
                                <br />
                                Set up your filters and click "Save current".
                            </div>
                        ) : (
                            views.map((view: any) => (
                                <div
                                    key={view.id}
                                    className={`group flex items-center gap-2 px-3 py-2 hover:bg-white/5 cursor-pointer transition-colors ${
                                        view.id === activeViewId ? 'bg-primary/10' : ''
                                    }`}
                                    onClick={() => handleLoad(view.id)}
                                >
                                    {editingId === view.id ? (
                                        <input
                                            type="text"
                                            value={editName}
                                            onChange={(e: any) => setEditName(e.target.value)}
                                            onKeyDown={(e) => handleRenameKeyDown(e, view.id)}
                                            onBlur={() => handleRename(view.id)}
                                            onClick={(e) => e.stopPropagation()}
                                            className="flex-1 px-2 py-0.5 bg-background border border-border rounded text-sm focus:outline-none focus:border-primary"
                                            autoFocus
                                        />
                                    ) : (
                                        <>
                                            <div className="relative shrink-0">
                                                <BookmarkSolidIcon className={`w-3.5 h-3.5 ${
                                                    view.id === activeViewId ? 'text-primary' : 'text-gray-500'
                                                }`} />
                                                {view.isDefault && (
                                                    <StarSolidIcon className="w-2 h-2 text-yellow-400 absolute -top-0.5 -right-0.5" />
                                                )}
                                            </div>
                                            <div className="flex-1 min-w-0">
                                                <div className="flex items-center gap-1.5">
                                                    <span className="text-sm text-gray-300 truncate">{view.name}</span>
                                                    {view.isDefault && (
                                                        <span className="text-[10px] text-yellow-400/70 uppercase tracking-wide">default</span>
                                                    )}
                                                </div>
                                                {view.query && (
                                                    <div className="text-xs text-gray-500 truncate">{view.query}</div>
                                                )}
                                            </div>
                                            <div className={`flex items-center gap-0.5 shrink-0 transition-opacity ${savedViewId === view.id ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'}`}>
                                                {onUpdate && (
                                                    <button
                                                        onClick={(e) => { e.stopPropagation(); handleUpdate(view.id); }}
                                                        className={`p-1 rounded transition-all ${
                                                            savedViewId === view.id
                                                                ? 'text-green-400 scale-110'
                                                                : 'text-gray-500 hover:text-primary hover:bg-white/10'
                                                        }`}
                                                        title="Save current settings to this view"
                                                    >
                                                        {savedViewId === view.id ? (
                                                            <CheckIcon className="w-3.5 h-3.5" />
                                                        ) : (
                                                            <ArrowDownTrayIcon className="w-3.5 h-3.5" />
                                                        )}
                                                    </button>
                                                )}
                                                {onSetDefault && (
                                                    <button
                                                        onClick={(e) => handleSetDefault(e, view.id)}
                                                        className={`p-1 rounded hover:bg-white/10 transition-colors ${
                                                            view.isDefault ? 'text-yellow-400 hover:text-yellow-300' : 'text-gray-500 hover:text-yellow-400'
                                                        }`}
                                                        title={view.isDefault ? 'Remove as default' : 'Set as default'}
                                                    >
                                                        {view.isDefault ? (
                                                            <StarSolidIcon className="w-3 h-3" />
                                                        ) : (
                                                            <StarIcon className="w-3 h-3" />
                                                        )}
                                                    </button>
                                                )}
                                                <button
                                                    onClick={(e) => handleStartEdit(e, view)}
                                                    className="p-1 text-gray-500 hover:text-white rounded hover:bg-white/10 transition-colors"
                                                    title="Rename"
                                                >
                                                    <PencilIcon className="w-3 h-3" />
                                                </button>
                                                {onDuplicate && (
                                                    <button
                                                        onClick={(e) => handleDuplicate(e, view.id)}
                                                        className="p-1 text-gray-500 hover:text-white rounded hover:bg-white/10 transition-colors"
                                                        title="Duplicate"
                                                    >
                                                        <DocumentDuplicateIcon className="w-3 h-3" />
                                                    </button>
                                                )}
                                                <button
                                                    onClick={(e) => handleDelete(e, view.id)}
                                                    className="p-1 text-gray-500 hover:text-red-400 rounded hover:bg-white/10 transition-colors"
                                                    title="Delete"
                                                >
                                                    <TrashIcon className="w-3 h-3" />
                                                </button>
                                            </div>
                                        </>
                                    )}
                                </div>
                            ))
                        )}
                    </div>

                    {/* Active view actions */}
                    {(hasActiveView || isDirty) && (
                        <div className="px-3 py-2 border-t border-border flex items-center gap-3">
                            {isDirty && (
                                <button
                                    onClick={() => { onLoad(activeViewId); setIsOpen(false); }}
                                    className="text-xs text-amber-400 hover:text-amber-300 transition-colors"
                                >
                                    Revert changes
                                </button>
                            )}
                            {hasActiveView && (
                                <button
                                    onClick={() => { onLoad(null); setIsOpen(false); }}
                                    className="text-xs text-gray-500 hover:text-gray-300 transition-colors"
                                >
                                    Clear active view
                                </button>
                            )}
                        </div>
                    )}
                </div>
            )}
        </div>
    );
}
