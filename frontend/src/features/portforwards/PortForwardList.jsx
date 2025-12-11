import React, { useMemo, useState, useCallback, useRef, useEffect } from 'react';
import {
    PlayIcon,
    StopIcon,
    TrashIcon,
    StarIcon,
    ArrowTopRightOnSquareIcon,
    PlusIcon,
    EllipsisVerticalIcon,
    PencilSquareIcon,
    MagnifyingGlassIcon
} from '@heroicons/react/24/outline';
import { StarIcon as StarIconSolid } from '@heroicons/react/24/solid';
import { usePortForwards } from '../../hooks/usePortForwards';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import PortForwardDialog from './PortForwardDialog';
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime';

const StatusText = ({ status }) => {
    const colors = {
        running: 'text-green-400',
        starting: 'text-yellow-400',
        stopped: 'text-gray-400',
        error: 'text-red-400'
    };

    return (
        <span className={`text-sm font-medium capitalize ${colors[status] || colors.stopped}`}>
            {status || 'stopped'}
        </span>
    );
};

// Column definitions
const ALL_COLUMNS = [
    { key: 'favorite', label: '', alwaysVisible: true },
    { key: 'label', label: 'Label' },
    { key: 'context', label: 'Context', contextOnly: true },
    { key: 'namespace', label: 'Namespace' },
    { key: 'type', label: 'Type' },
    { key: 'resource', label: 'Resource' },
    { key: 'localPort', label: 'Local Port' },
    { key: 'remotePort', label: 'Remote Port' },
    { key: 'status', label: 'Status' },
    { key: 'actions', label: '', alwaysVisible: true, isActions: true }
];

export default function PortForwardList({ isVisible }) {
    const { currentContext, contexts } = useK8s();
    const { openModal, closeModal, navigateWithSearch } = useUI();
    const [showAllContexts, setShowAllContexts] = useState(false);
    const [dialogOpen, setDialogOpen] = useState(false);
    const [editingConfig, setEditingConfig] = useState(null);
    const [hiddenColumns, setHiddenColumns] = useState(new Set(['type']));
    const [showColumnMenu, setShowColumnMenu] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const columnMenuRef = useRef(null);
    const searchInputRef = useRef(null);

    // Filter by current context unless showing all
    const contextFilter = showAllContexts ? '' : currentContext;

    // Handle click outside column menu
    useEffect(() => {
        const handleClickOutside = (event) => {
            if (columnMenuRef.current && !columnMenuRef.current.contains(event.target)) {
                setShowColumnMenu(false);
            }
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, []);

    // Get visible columns based on settings
    const visibleColumns = useMemo(() => {
        return ALL_COLUMNS.filter(col => {
            if (col.alwaysVisible) return true;
            if (col.contextOnly && !showAllContexts) return false;
            return !hiddenColumns.has(col.key);
        });
    }, [hiddenColumns, showAllContexts]);

    const toggleColumn = useCallback((key) => {
        setHiddenColumns(prev => {
            const next = new Set(prev);
            if (next.has(key)) {
                next.delete(key);
            } else {
                next.add(key);
            }
            return next;
        });
    }, []);

    const {
        configs,
        loading,
        error,
        addConfig,
        updateConfig,
        deleteConfig,
        startForward,
        stopForward,
        isActive,
        getStatus,
        getError
    } = usePortForwards(contextFilter, isVisible);

    const handleAdd = useCallback(() => {
        setEditingConfig(null);
        setDialogOpen(true);
    }, []);

    const handleViewResource = useCallback((config) => {
        // Navigate to the resource view with a search filter
        const viewMap = {
            pod: 'pods',
            service: 'services'
        };
        const view = viewMap[config.resourceType] || 'pods';
        const searchTerm = `name:"${config.resourceName}" namespace:"${config.namespace}"`;
        navigateWithSearch(view, searchTerm);
    }, [navigateWithSearch]);

    const handleEditConfig = useCallback((config) => {
        setEditingConfig(config);
        setDialogOpen(true);
    }, []);

    const handleDelete = useCallback((config) => {
        openModal({
            title: 'Delete Port Forward',
            content: `Are you sure you want to delete the port forward "${config.label || config.resourceName}"?`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    // Stop the forward first if it's running
                    if (isActive(config.id)) {
                        await stopForward(config.id);
                    }
                    await deleteConfig(config.id);
                    closeModal();
                } catch (err) {
                    console.error('Failed to delete:', err);
                }
            }
        });
    }, [deleteConfig, stopForward, isActive, openModal, closeModal]);

    const handleToggle = useCallback(async (config) => {
        try {
            if (isActive(config.id)) {
                await stopForward(config.id);
            } else {
                await startForward(config.id);
            }
        } catch (err) {
            console.error('Failed to toggle port forward:', err);
        }
    }, [isActive, startForward, stopForward]);

    const handleToggleFavorite = useCallback(async (config) => {
        try {
            await updateConfig({ ...config, favorite: !config.favorite });
        } catch (err) {
            console.error('Failed to toggle favorite:', err);
        }
    }, [updateConfig]);

    const handleOpenBrowser = useCallback((config) => {
        const protocol = config.https ? 'https' : 'http';
        const url = `${protocol}://localhost:${config.localPort}`;
        window.open(url, '_blank');
    }, []);

    const handleSave = useCallback(async (config) => {
        try {
            if (editingConfig) {
                // Editing existing config
                await updateConfig(config);
            } else {
                // Adding new config
                const result = await addConfig(config);
                // Auto-start if requested
                if (config.autoStart && result?.id) {
                    try {
                        await startForward(result.id);
                        // Open in browser if requested
                        if (config.openInBrowser) {
                            const protocol = config.https ? 'https' : 'http';
                            BrowserOpenURL(`${protocol}://localhost:${config.localPort}`);
                        }
                    } catch (startErr) {
                        console.error('Failed to auto-start port forward:', startErr);
                    }
                }
            }
            setDialogOpen(false);
            setEditingConfig(null);
        } catch (err) {
            throw err; // Re-throw to let dialog handle it
        }
    }, [editingConfig, addConfig, updateConfig, startForward]);

    // Filter and sort configs: search filter, then favorites first, then by name
    const filteredAndSortedConfigs = useMemo(() => {
        let result = [...configs];

        // Apply search filter
        if (searchTerm.trim()) {
            const term = searchTerm.toLowerCase();
            result = result.filter(config => {
                const label = (config.label || '').toLowerCase();
                const resource = (config.resourceName || '').toLowerCase();
                const namespace = (config.namespace || '').toLowerCase();
                const context = (config.context || '').toLowerCase();
                const localPort = String(config.localPort || '');
                const remotePort = String(config.remotePort || '');
                const type = (config.resourceType || '').toLowerCase();

                return label.includes(term) ||
                       resource.includes(term) ||
                       namespace.includes(term) ||
                       context.includes(term) ||
                       localPort.includes(term) ||
                       remotePort.includes(term) ||
                       type.includes(term);
            });
        }

        // Sort: favorites first, then by label/name
        return result.sort((a, b) => {
            if (a.favorite !== b.favorite) return a.favorite ? -1 : 1;
            const nameA = a.label || a.resourceName;
            const nameB = b.label || b.resourceName;
            return nameA.localeCompare(nameB);
        });
    }, [configs, searchTerm]);

    return (
        <div className="h-full flex flex-col">
            {/* Header */}
            <div className="h-14 border-b border-border flex items-center justify-between px-4 bg-surface shrink-0 gap-4">
                <div className="flex items-center gap-4 flex-1">
                    <h1 className="text-lg font-semibold text-text shrink-0">Port Forwards</h1>

                    {/* Search Bar */}
                    <div className="relative max-w-md w-full">
                        <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-3 top-1/2 transform -translate-y-1/2" />
                        <input
                            ref={searchInputRef}
                            type="text"
                            value={searchTerm}
                            onChange={(e) => setSearchTerm(e.target.value)}
                            placeholder="Search port forwards..."
                            className="w-full bg-background border border-border rounded-md pl-9 pr-4 py-1.5 text-sm text-text focus:outline-none focus:border-primary transition-colors"
                            autoComplete="off"
                            autoCorrect="off"
                            spellCheck="false"
                        />
                    </div>

                    <span className="text-sm text-gray-400 shrink-0">
                        {searchTerm ? `${filteredAndSortedConfigs.length} of ${configs.length}` : configs.length} {configs.length === 1 ? 'forward' : 'forwards'}
                    </span>
                </div>
                <div className="flex items-center gap-4">
                    <label className="flex items-center gap-2 text-sm text-gray-400 cursor-pointer">
                        <input
                            type="checkbox"
                            checked={showAllContexts}
                            onChange={(e) => setShowAllContexts(e.target.checked)}
                            className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                        />
                        Show all contexts
                    </label>
                    <button
                        onClick={handleAdd}
                        className="flex items-center gap-2 px-3 py-1.5 text-sm bg-primary hover:bg-primary/80 text-white rounded transition-colors"
                    >
                        <PlusIcon className="w-4 h-4" />
                        Add Port Forward
                    </button>
                </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto">
                {loading ? (
                    <div className="flex items-center justify-center h-32">
                        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                    </div>
                ) : error ? (
                    <div className="flex items-center justify-center h-32 text-red-400">
                        Failed to load port forwards
                    </div>
                ) : configs.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-64 text-gray-400">
                        <p className="text-lg mb-2">No port forwards configured</p>
                        <p className="text-sm mb-4">Create a port forward to access services running in your cluster</p>
                        <button
                            onClick={handleAdd}
                            className="flex items-center gap-2 px-4 py-2 bg-primary hover:bg-primary/80 text-white rounded transition-colors"
                        >
                            <PlusIcon className="w-4 h-4" />
                            Add Port Forward
                        </button>
                    </div>
                ) : filteredAndSortedConfigs.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-64 text-gray-400">
                        <p className="text-lg mb-2">No matching port forwards</p>
                        <p className="text-sm">Try adjusting your search term</p>
                    </div>
                ) : (
                    <table className="w-full">
                        <thead className="bg-surface-light sticky top-0">
                            <tr className="text-left text-xs font-semibold text-gray-400 uppercase tracking-wider">
                                {visibleColumns.map((col) => (
                                    <th
                                        key={col.key}
                                        className={`px-6 py-3 ${col.key === 'favorite' ? 'w-8' : ''} ${col.isActions ? 'text-right' : ''}`}
                                    >
                                        {col.isActions ? (
                                            <div className="relative flex justify-end" ref={columnMenuRef}>
                                                <button
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        setShowColumnMenu(!showColumnMenu);
                                                    }}
                                                    className="p-1 hover:bg-white/10 rounded transition-colors"
                                                    title="Configure columns"
                                                >
                                                    <EllipsisVerticalIcon className="h-5 w-5" />
                                                </button>
                                                {showColumnMenu && (
                                                    <div className="absolute right-0 top-full mt-1 w-48 bg-surface border border-border rounded-md shadow-lg z-50 py-1">
                                                        {ALL_COLUMNS.filter(c => !c.alwaysVisible && (!c.contextOnly || showAllContexts)).map(c => (
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
                                            col.label
                                        )}
                                    </th>
                                ))}
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-border">
                            {filteredAndSortedConfigs.map((config) => {
                                const status = getStatus(config.id);
                                const configError = getError(config.id);
                                const running = status === 'running';

                                return (
                                    <tr key={config.id} className="hover:bg-white/5 transition-colors">
                                        {visibleColumns.map((col) => {
                                            switch (col.key) {
                                                case 'favorite':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3">
                                                            <button
                                                                onClick={() => handleToggleFavorite(config)}
                                                                className="text-gray-400 hover:text-yellow-400 transition-colors"
                                                                title={config.favorite ? 'Remove from favorites' : 'Add to favorites'}
                                                            >
                                                                {config.favorite ? (
                                                                    <StarIconSolid className="w-4 h-4 text-yellow-400" />
                                                                ) : (
                                                                    <StarIcon className="w-4 h-4" />
                                                                )}
                                                            </button>
                                                        </td>
                                                    );
                                                case 'label':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3 text-sm">
                                                            {config.label || config.resourceName}
                                                        </td>
                                                    );
                                                case 'context':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3 text-sm text-gray-400">
                                                            {config.context}
                                                        </td>
                                                    );
                                                case 'namespace':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3 text-sm text-gray-400">
                                                            {config.namespace}
                                                        </td>
                                                    );
                                                case 'type':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3 text-sm">
                                                            <span className="capitalize">{config.resourceType}</span>
                                                        </td>
                                                    );
                                                case 'resource':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3 text-sm">
                                                            <button
                                                                onClick={() => handleViewResource(config)}
                                                                className="text-primary hover:text-primary/80 hover:underline transition-colors"
                                                                title={`View ${config.resourceType}`}
                                                            >
                                                                {config.resourceName}
                                                            </button>
                                                        </td>
                                                    );
                                                case 'localPort':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3 text-sm font-mono">
                                                            {config.localPort}
                                                        </td>
                                                    );
                                                case 'remotePort':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3 text-sm font-mono">
                                                            {config.remotePort}
                                                        </td>
                                                    );
                                                case 'status':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3">
                                                            <div className="flex flex-col gap-1">
                                                                <StatusText status={status} />
                                                                {configError && (
                                                                    <span className="text-xs text-red-400 truncate block" title={configError}>
                                                                        {configError}
                                                                    </span>
                                                                )}
                                                            </div>
                                                        </td>
                                                    );
                                                case 'actions':
                                                    return (
                                                        <td key={col.key} className="px-6 py-3">
                                                            <div className="flex items-center justify-end gap-1">
                                                                <button
                                                                    onClick={() => handleToggle(config)}
                                                                    className={`p-1.5 rounded transition-colors ${
                                                                        running
                                                                            ? 'text-red-400 hover:bg-red-500/20'
                                                                            : 'text-green-400 hover:bg-green-500/20'
                                                                    }`}
                                                                    title={running ? 'Stop' : 'Start'}
                                                                >
                                                                    {running ? (
                                                                        <StopIcon className="w-4 h-4" />
                                                                    ) : (
                                                                        <PlayIcon className="w-4 h-4" />
                                                                    )}
                                                                </button>
                                                                <button
                                                                    onClick={() => handleOpenBrowser(config)}
                                                                    className={`p-1.5 rounded transition-colors ${
                                                                        running
                                                                            ? 'text-gray-400 hover:text-white hover:bg-white/10'
                                                                            : 'text-gray-600 cursor-not-allowed'
                                                                    }`}
                                                                    title={running ? `Open ${config.https ? 'https' : 'http'}://localhost:${config.localPort}` : 'Start to open in browser'}
                                                                    disabled={!running}
                                                                >
                                                                    <ArrowTopRightOnSquareIcon className="w-4 h-4" />
                                                                </button>
                                                                <button
                                                                    onClick={() => handleEditConfig(config)}
                                                                    className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                                                                    title="Edit port forward"
                                                                >
                                                                    <PencilSquareIcon className="w-4 h-4" />
                                                                </button>
                                                                <button
                                                                    onClick={() => handleDelete(config)}
                                                                    className="p-1.5 text-gray-400 hover:text-red-400 hover:bg-red-500/10 rounded transition-colors"
                                                                    title="Delete"
                                                                >
                                                                    <TrashIcon className="w-4 h-4" />
                                                                </button>
                                                            </div>
                                                        </td>
                                                    );
                                                default:
                                                    return null;
                                            }
                                        })}
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                )}
            </div>

            {/* Dialog */}
            <PortForwardDialog
                open={dialogOpen}
                onOpenChange={(open) => {
                    setDialogOpen(open);
                    if (!open) setEditingConfig(null);
                }}
                config={editingConfig}
                onSave={handleSave}
                contexts={contexts}
                currentContext={currentContext}
            />
        </div>
    );
}
