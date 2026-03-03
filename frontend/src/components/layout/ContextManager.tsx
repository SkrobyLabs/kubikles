import React, { useState, useEffect, useCallback, useMemo } from 'react';
import { createPortal } from 'react-dom';
import {
    TrashIcon,
    PencilSquareIcon,
    XMarkIcon,
    PlusIcon,
    FolderOpenIcon,
    CheckIcon,
    MagnifyingGlassIcon,
} from '@heroicons/react/24/outline';
import { useNotification } from '~/context';
import { useConfig } from '~/context';
import {
    GetContextDetails,
    DeleteContext,
    RenameContext,
    SelectKubeconfigFile,
} from 'wailsjs/go/main/App';

interface ContextDetail {
    name: string;
    cluster: string;
    server: string;
    authInfo: string;
    namespace: string;
    isActive: boolean;
}

interface ContextManagerProps {
    onClose: () => void;
    onContextsChanged: () => void;
}

export default function ContextManager({ onClose, onContextsChanged }: ContextManagerProps) {
    const [contexts, setContexts] = useState<ContextDetail[]>([]);
    const [loading, setLoading] = useState(true);
    const [renamingContext, setRenamingContext] = useState<string | null>(null);
    const [renameValue, setRenameValue] = useState('');
    const [deletingContext, setDeletingContext] = useState<string | null>(null);
    const [actionLoading, setActionLoading] = useState(false);
    const [searchQuery, setSearchQuery] = useState('');
    const { addNotification } = useNotification();
    const { config, setConfig } = useConfig();

    const extraPaths: string[] = config?.kubernetes?.extraKubeconfigPaths || [];

    const filteredContexts = useMemo(() => {
        if (!searchQuery.trim()) return contexts;
        const q = searchQuery.toLowerCase();
        return contexts.filter(ctx =>
            ctx.name.toLowerCase().includes(q) ||
            ctx.server.toLowerCase().includes(q) ||
            ctx.cluster.toLowerCase().includes(q)
        );
    }, [contexts, searchQuery]);

    const fetchContexts = useCallback(async () => {
        try {
            const details = await GetContextDetails();
            setContexts((details || []).sort((a: ContextDetail, b: ContextDetail) => a.name.localeCompare(b.name)));
        } catch (err: any) {
            addNotification({ type: 'error', title: 'Failed to load contexts', message: String(err) });
        } finally {
            setLoading(false);
        }
    }, [addNotification]);

    useEffect(() => {
        fetchContexts();
    }, [fetchContexts]);

    const handleDelete = async (name: string) => {
        setActionLoading(true);
        try {
            await DeleteContext(name);
            addNotification({ type: 'success', title: 'Context deleted', message: `Removed "${name}"` });
            setDeletingContext(null);
            await fetchContexts();
            onContextsChanged();
        } catch (err: any) {
            addNotification({ type: 'error', title: 'Failed to delete context', message: String(err) });
        } finally {
            setActionLoading(false);
        }
    };

    const handleRename = async (oldName: string) => {
        const newName = renameValue.trim();
        if (!newName || newName === oldName) {
            setRenamingContext(null);
            return;
        }
        setActionLoading(true);
        try {
            await RenameContext(oldName, newName);
            addNotification({ type: 'success', title: 'Context renamed', message: `"${oldName}" → "${newName}"` });
            setRenamingContext(null);
            await fetchContexts();
            onContextsChanged();
        } catch (err: any) {
            addNotification({ type: 'error', title: 'Failed to rename context', message: String(err) });
        } finally {
            setActionLoading(false);
        }
    };

    const handleAddPath = async () => {
        try {
            const path = await SelectKubeconfigFile();
            if (!path) return; // User canceled
            if (extraPaths.includes(path)) {
                addNotification({ type: 'info', title: 'Already added', message: `"${path}" is already in the list` });
                return;
            }
            const updated = [...extraPaths, path];
            setConfig('kubernetes.extraKubeconfigPaths', updated);
            addNotification({ type: 'success', title: 'Kubeconfig added', message: path });
            // Refresh contexts after a brief delay to allow config to propagate
            setTimeout(async () => {
                await fetchContexts();
                onContextsChanged();
            }, 200);
        } catch (err: any) {
            addNotification({ type: 'error', title: 'Failed to add kubeconfig', message: String(err) });
        }
    };

    const handleRemovePath = (path: string) => {
        const updated = extraPaths.filter(p => p !== path);
        setConfig('kubernetes.extraKubeconfigPaths', updated);
        addNotification({ type: 'success', title: 'Kubeconfig removed', message: path });
        setTimeout(async () => {
            await fetchContexts();
            onContextsChanged();
        }, 200);
    };

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
            <div className="absolute inset-0 bg-black/50" onClick={onClose} />
            <div className="relative bg-surface-light border border-border rounded-lg shadow-xl max-w-2xl w-full mx-4 max-h-[80vh] flex flex-col">
                {/* Header */}
                <div className="flex items-center justify-between px-4 py-3 border-b border-border">
                    <h2 className="text-lg font-medium text-white">Context Manager</h2>
                    <button
                        onClick={onClose}
                        className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors"
                    >
                        <XMarkIcon className="h-5 w-5" />
                    </button>
                </div>

                {/* Search */}
                <div className="px-4 pt-3 pb-0 shrink-0">
                    <div className="relative">
                        <MagnifyingGlassIcon className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-gray-500" />
                        <input
                            type="text"
                            value={searchQuery}
                            onChange={e => setSearchQuery(e.target.value)}
                            placeholder="Filter contexts..."
                            autoComplete="off"
                            autoCorrect="off"
                            autoCapitalize="off"
                            spellCheck={false}
                            autoFocus
                            className="w-full bg-surface border border-border rounded-md pl-8 pr-3 py-1.5 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-primary"
                        />
                    </div>
                </div>

                {/* Content */}
                <div className="flex-1 overflow-y-auto p-4 space-y-6">
                    {/* Contexts list */}
                    <div>
                        <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-2">Contexts</h3>
                        {loading ? (
                            <div className="flex items-center gap-2 text-sm text-gray-400 py-4">
                                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary"></div>
                                Loading contexts...
                            </div>
                        ) : filteredContexts.length === 0 ? (
                            <p className="text-sm text-gray-500 py-2">{searchQuery ? 'No matching contexts' : 'No contexts found'}</p>
                        ) : (
                            <div className="space-y-1">
                                {filteredContexts.map(ctx => (
                                    <div
                                        key={ctx.name}
                                        className="flex items-center gap-2 px-3 py-2 rounded-md bg-surface hover:bg-surface-hover transition-colors group"
                                    >
                                        {/* Active indicator */}
                                        <div className={`w-2 h-2 rounded-full shrink-0 ${ctx.isActive ? 'bg-green-500' : 'bg-transparent'}`} />

                                        {/* Name / rename input */}
                                        <div className="flex-1 min-w-0">
                                            {renamingContext === ctx.name ? (
                                                <div className="flex items-center gap-1">
                                                    <input
                                                        type="text"
                                                        value={renameValue}
                                                        onChange={e => setRenameValue(e.target.value)}
                                                        onKeyDown={e => {
                                                            if (e.key === 'Enter') handleRename(ctx.name);
                                                            if (e.key === 'Escape') setRenamingContext(null);
                                                        }}
                                                        autoFocus
                                                        className="bg-surface-light border border-border rounded px-2 py-0.5 text-sm text-white w-full focus:outline-none focus:border-primary"
                                                    />
                                                    <button
                                                        onClick={() => handleRename(ctx.name)}
                                                        disabled={actionLoading}
                                                        className="p-1 rounded hover:bg-white/10 text-green-400 hover:text-green-300 transition-colors"
                                                    >
                                                        <CheckIcon className="h-4 w-4" />
                                                    </button>
                                                    <button
                                                        onClick={() => setRenamingContext(null)}
                                                        className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors"
                                                    >
                                                        <XMarkIcon className="h-4 w-4" />
                                                    </button>
                                                </div>
                                            ) : (
                                                <>
                                                    <div className="text-sm text-white truncate">{ctx.name}</div>
                                                    <div className="text-xs text-gray-500 truncate">{ctx.server || 'No server'}</div>
                                                </>
                                            )}
                                        </div>

                                        {/* Actions */}
                                        {renamingContext !== ctx.name && (
                                            <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity shrink-0">
                                                <button
                                                    onClick={() => {
                                                        setRenamingContext(ctx.name);
                                                        setRenameValue(ctx.name);
                                                    }}
                                                    className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors"
                                                    title="Rename context"
                                                >
                                                    <PencilSquareIcon className="h-4 w-4" />
                                                </button>
                                                {ctx.isActive ? (
                                                    <button
                                                        disabled
                                                        className="p-1 rounded text-gray-600 cursor-not-allowed"
                                                        title="Cannot delete the active context"
                                                    >
                                                        <TrashIcon className="h-4 w-4" />
                                                    </button>
                                                ) : deletingContext === ctx.name ? (
                                                    <div className="flex items-center gap-1">
                                                        <button
                                                            onClick={() => handleDelete(ctx.name)}
                                                            disabled={actionLoading}
                                                            className="px-2 py-0.5 text-xs rounded bg-red-600 hover:bg-red-700 text-white transition-colors disabled:opacity-50"
                                                        >
                                                            Delete
                                                        </button>
                                                        <button
                                                            onClick={() => setDeletingContext(null)}
                                                            className="px-2 py-0.5 text-xs rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                                                        >
                                                            Cancel
                                                        </button>
                                                    </div>
                                                ) : (
                                                    <button
                                                        onClick={() => setDeletingContext(ctx.name)}
                                                        className="p-1 rounded hover:bg-red-500/10 text-gray-400 hover:text-red-400 transition-colors"
                                                        title="Delete context"
                                                    >
                                                        <TrashIcon className="h-4 w-4" />
                                                    </button>
                                                )}
                                            </div>
                                        )}
                                    </div>
                                ))}
                            </div>
                        )}
                    </div>

                    {/* Kubeconfig Paths */}
                    <div>
                        <h3 className="text-sm font-semibold text-gray-400 uppercase tracking-wider mb-2">Kubeconfig Files</h3>
                        <div className="space-y-1">
                            {/* Primary path (non-removable) */}
                            <div className="flex items-center gap-2 px-3 py-2 rounded-md bg-surface text-sm">
                                <FolderOpenIcon className="h-4 w-4 text-gray-500 shrink-0" />
                                <span className="text-gray-300 truncate flex-1">~/.kube/config</span>
                                <span className="text-xs text-gray-600 shrink-0">default</span>
                            </div>

                            {/* Extra paths */}
                            {extraPaths.map(path => (
                                <div key={path} className="flex items-center gap-2 px-3 py-2 rounded-md bg-surface hover:bg-surface-hover transition-colors group text-sm">
                                    <FolderOpenIcon className="h-4 w-4 text-gray-500 shrink-0" />
                                    <span className="text-gray-300 truncate flex-1">{path}</span>
                                    <button
                                        onClick={() => handleRemovePath(path)}
                                        className="p-0.5 rounded hover:bg-red-500/10 text-gray-500 hover:text-red-400 transition-colors opacity-0 group-hover:opacity-100 shrink-0"
                                        title="Remove kubeconfig file"
                                    >
                                        <XMarkIcon className="h-4 w-4" />
                                    </button>
                                </div>
                            ))}

                            {/* Add button */}
                            <button
                                onClick={handleAddPath}
                                className="flex items-center gap-2 px-3 py-2 rounded-md text-sm text-gray-400 hover:text-white hover:bg-white/5 transition-colors w-full"
                            >
                                <PlusIcon className="h-4 w-4" />
                                Add kubeconfig file...
                            </button>
                        </div>
                    </div>
                </div>

                {/* Footer */}
                <div className="flex justify-end px-4 py-3 border-t border-border">
                    <button
                        onClick={onClose}
                        className="px-4 py-2 text-sm text-gray-300 hover:text-white hover:bg-surface-hover rounded transition-colors"
                    >
                        Close
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
}
