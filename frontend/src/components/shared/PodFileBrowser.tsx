import React, { useState, useEffect, useCallback, useRef } from 'react';
import {
    ListPodFiles,
    DownloadPodFile,
    DownloadPodFolder,
    DownloadPodFiles,
    UploadToPod,
    UploadFileToPod,
    CreatePodDirectory,
    DeletePodFile
} from 'wailsjs/go/main/App';
import { EventsOn, EventsOff, OnFileDrop, OnFileDropOff } from 'wailsjs/runtime/runtime';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import ContainerSelector from './ContainerSelector';
import SearchSelect from './SearchSelect';
import Tooltip from './Tooltip';
import {
    FolderIcon,
    DocumentIcon,
    ArrowUpTrayIcon,
    ArrowDownTrayIcon,
    ArrowPathIcon,
    FolderPlusIcon,
    TrashIcon,
    ChevronRightIcon,
    ExclamationTriangleIcon,
    HomeIcon
} from '@heroicons/react/24/outline';

// Format bytes to human readable
function formatBytes(bytes: any) {
    if (bytes === 0) return '0 B';
    if (bytes < 0) return '-';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
}

// Progress bar component
function ProgressBar({ progress }: { progress: any }) {
    if (!progress || progress.done) return null;

    const percentage = progress.totalBytes > 0
        ? Math.round((progress.bytesTransferred / progress.totalBytes) * 100)
        : null;

    return (
        <div className="absolute bottom-0 left-0 right-0 bg-surface border-t border-border p-3">
            <div className="flex items-center gap-3">
                <div className="flex-1">
                    <div className="flex justify-between text-xs text-gray-400 mb-1">
                        <span>{progress.operation === 'upload' ? 'Uploading' : 'Downloading'}: {progress.fileName}</span>
                        <span>
                            {formatBytes(progress.bytesTransferred)}
                            {progress.totalBytes > 0 && ` / ${formatBytes(progress.totalBytes)}`}
                        </span>
                    </div>
                    <div className="h-1.5 bg-background rounded-full overflow-hidden">
                        {percentage !== null ? (
                            <div
                                className="h-full bg-primary transition-all duration-150"
                                style={{ width: `${percentage}%` }}
                            />
                        ) : (
                            <div className="h-full bg-primary animate-pulse" style={{ width: '100%' }} />
                        )}
                    </div>
                </div>
                {percentage !== null && (
                    <span className="text-xs text-gray-400 w-10 text-right">{percentage}%</span>
                )}
            </div>
        </div>
    );
}

// Helper to get container name from container (supports both string and object format)
const getContainerName = (container: any) => {
    return typeof container === 'object' ? container.name : container;
};

export default function PodFileBrowser({
    namespace,
    pod,
    containers = [],
    tabContext = ''
}: any) {
    const { currentContext } = useK8s();
    const { openModal, closeModal } = useUI();
    // Show container selector initially if multiple containers available
    const [state, setState] = useState(containers.length > 1 ? 'selecting' : 'browsing');
    const [currentPath, setCurrentPath] = useState('/');
    const [files, setFiles] = useState<any[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<any>(null);
    const [selectedContainer, setSelectedContainer] = useState(
        containers.length === 1 ? getContainerName(containers[0]) : ''
    );
    const [selectedFiles, setSelectedFiles] = useState(new Set<any>());
    const [progress, setProgress] = useState<any>(null);
    const [isDragging, setIsDragging] = useState(false);
    const [showNewFolderInput, setShowNewFolderInput] = useState(false);
    const [newFolderName, setNewFolderName] = useState('');
    const dropZoneRef = useRef<any>(null);
    const newFolderInputRef = useRef<any>(null);
    const dragCounterRef = useRef(0);
    const dragLeaveTimeoutRef = useRef<any>(null);

    const isStale = tabContext && tabContext !== currentContext;

    // Load files when path or container changes
    const loadFiles = useCallback(async () => {
        if (!namespace || !pod || !selectedContainer || state !== 'browsing') return;

        setLoading(true);
        setError(null);
        setSelectedFiles(new Set<any>());

        try {
            const result = await ListPodFiles(namespace, pod, selectedContainer, currentPath);
            setFiles(result || []);
        } catch (err: any) {
            console.error('Failed to list files:', err);
            setError(err.message || String(err));
            setFiles([]);
        } finally {
            setLoading(false);
        }
    }, [namespace, pod, selectedContainer, currentPath, state]);

    useEffect(() => {
        if (state === 'browsing') {
            loadFiles();
        }
    }, [loadFiles, state]);

    // Handle container selection and start browsing
    const handleContainerSelect = (container: any) => {
        setSelectedContainer(container);
        setState('browsing');
    };

    // Listen for progress events
    useEffect(() => {
        const handleProgress = (data: any) => {
            setProgress(data);
            if (data.done) {
                // Clear progress after a short delay and refresh
                setTimeout(() => {
                    setProgress(null);
                    if (!data.error) {
                        loadFiles();
                    }
                }, 1000);
            }
        };

        EventsOn('file:progress', handleProgress);
        return () => EventsOff('file:progress');
    }, [loadFiles]);

    // Set up drag and drop using Wails native file drop
    useEffect(() => {
        if (isStale) return;

        const handleFileDrop = async (x: any, y: any, paths: any) => {
            setIsDragging(false);
            dragCounterRef.current = 0;

            if (paths && paths.length > 0) {
                // Upload each dropped file sequentially
                for (const localPath of paths) {
                    try {
                        await UploadFileToPod(namespace, pod, selectedContainer, localPath, currentPath);
                    } catch (err: any) {
                        console.error('Upload failed:', err);
                    }
                }
            }
        };

        // Register Wails file drop handler
        // useDropTarget=false means it works window-wide when this component is mounted
        OnFileDrop(handleFileDrop, false);

        return () => {
            OnFileDropOff();
        };
    }, [namespace, pod, selectedContainer, currentPath, isStale]);

    // Handle drag events for visual feedback only
    // These don't interfere with Wails native drop handling
    // Use debounced approach to prevent window glitching from rapid event firing
    const handleDragEnter = useCallback((e: any) => {
        e.preventDefault();
        e.stopPropagation();

        // Clear any pending hide timeout
        if (dragLeaveTimeoutRef.current) {
            clearTimeout(dragLeaveTimeoutRef.current);
            dragLeaveTimeoutRef.current = null;
        }

        dragCounterRef.current++;
        if (!isStale && !isDragging) {
            setIsDragging(true);
        }
    }, [isStale, isDragging]);

    const handleDragOver = useCallback((e: any) => {
        e.preventDefault();
        e.stopPropagation();
        // Keep the drag indicator alive
        if (dragLeaveTimeoutRef.current) {
            clearTimeout(dragLeaveTimeoutRef.current);
            dragLeaveTimeoutRef.current = null;
        }
    }, []);

    const handleDragLeave = useCallback((e: any) => {
        e.preventDefault();
        e.stopPropagation();
        dragCounterRef.current--;

        // Debounce the drag leave - wait a bit before hiding
        // This prevents flickering when crossing element boundaries
        if (dragCounterRef.current <= 0) {
            dragCounterRef.current = 0;
            if (dragLeaveTimeoutRef.current) {
                clearTimeout(dragLeaveTimeoutRef.current);
            }
            dragLeaveTimeoutRef.current = setTimeout(() => {
                setIsDragging(false);
                dragLeaveTimeoutRef.current = null;
            }, 50);
        }
    }, []);

    const handleDrop = useCallback((e: any) => {
        e.preventDefault();
        e.stopPropagation();

        if (dragLeaveTimeoutRef.current) {
            clearTimeout(dragLeaveTimeoutRef.current);
            dragLeaveTimeoutRef.current = null;
        }

        dragCounterRef.current = 0;
        setIsDragging(false);
        // Wails OnFileDrop handles the actual file processing
    }, []);

    // Cleanup timeout on unmount
    useEffect(() => {
        return () => {
            if (dragLeaveTimeoutRef.current) {
                clearTimeout(dragLeaveTimeoutRef.current);
            }
        };
    }, []);

    // Navigation
    const navigateTo = (path: any) => {
        setCurrentPath(path);
    };

    const navigateUp = () => {
        if (currentPath === '/') return;
        const parts = currentPath.split('/').filter(Boolean);
        parts.pop();
        setCurrentPath('/' + parts.join('/'));
    };

    const handleFileDoubleClick = (file: any) => {
        if (file.isDir) {
            if (file.name === '..') {
                navigateUp();
            } else {
                const newPath = currentPath === '/'
                    ? '/' + file.name
                    : currentPath + '/' + file.name;
                navigateTo(newPath);
            }
        }
    };

    const toggleFileSelection = (file: any, e?: any) => {
        if (e) e.stopPropagation();
        if (file.name === '..') return;
        setSelectedFiles(prev => {
            const next = new Set(prev);
            if (next.has(file.name)) {
                next.delete(file.name);
            } else {
                next.add(file.name);
            }
            return next;
        });
    };

    const selectableFiles = files.filter((f: any) => f.name !== '..');

    const toggleSelectAll = () => {
        if (selectedFiles.size === selectableFiles.length) {
            setSelectedFiles(new Set<any>());
        } else {
            setSelectedFiles(new Set(selectableFiles.map((f: any) => f.name)));
        }
    };

    // Get selected file objects
    const getSelectedFileObjects = () => {
        return files.filter((f: any) => selectedFiles.has(f.name));
    };

    // Actions
    const handleDownload = async () => {
        const selected = getSelectedFileObjects();
        if (selected.length === 0) return;

        try {
            if (selected.length > 1) {
                // Multiple items: batch download as single tar.gz
                await DownloadPodFiles(namespace, pod, selectedContainer, currentPath, selected.map((f: any) => f.name));
            } else {
                // Single item: use specific handler
                const file = selected[0];
                const remotePath = currentPath === '/'
                    ? '/' + file.name
                    : currentPath + '/' + file.name;

                if (file.isDir) {
                    await DownloadPodFolder(namespace, pod, selectedContainer, remotePath);
                } else {
                    await DownloadPodFile(namespace, pod, selectedContainer, remotePath);
                }
            }
        } catch (err: any) {
            console.error('Download failed:', err);
        }
    };

    const handleUpload = async () => {
        try {
            await UploadToPod(namespace, pod, selectedContainer, currentPath + '/');
        } catch (err: any) {
            console.error('Upload failed:', err);
        }
    };

    const handleCreateFolder = async () => {
        if (!newFolderName.trim()) return;

        const dirPath = currentPath === '/'
            ? '/' + newFolderName.trim()
            : currentPath + '/' + newFolderName.trim();

        try {
            await CreatePodDirectory(namespace, pod, selectedContainer, dirPath);
            setNewFolderName('');
            setShowNewFolderInput(false);
            loadFiles();
        } catch (err: any) {
            console.error('Failed to create directory:', err);
        }
    };

    const handleDelete = () => {
        const selected = getSelectedFileObjects();
        if (selected.length === 0) return;

        const message = selected.length === 1
            ? `Delete "${selected[0].name}"? This cannot be undone.`
            : `Delete ${selected.length} items? This cannot be undone.`;

        openModal({
            title: 'Confirm Delete',
            content: message,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                closeModal();
                setLoading(true);
                try {
                    for (const file of selected) {
                        const filePath = currentPath === '/'
                            ? '/' + file.name
                            : currentPath + '/' + file.name;
                        await DeletePodFile(namespace, pod, selectedContainer, filePath);
                    }
                    setSelectedFiles(new Set<any>());
                    await loadFiles();
                } catch (err: any) {
                    console.error('Failed to delete:', err);
                    setError('Failed to delete: ' + (err.message || String(err)));
                } finally {
                    setLoading(false);
                }
            }
        });
    };

    // Focus new folder input when shown
    useEffect(() => {
        if (showNewFolderInput && newFolderInputRef.current) {
            newFolderInputRef.current.focus();
        }
    }, [showNewFolderInput]);

    // Build breadcrumb
    const pathParts = currentPath.split('/').filter(Boolean);

    return (
        <div
            className="flex flex-col h-full bg-background relative"
            onDragEnter={handleDragEnter}
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
            ref={dropZoneRef}
        >
            {/* Container Selection View */}
            {state === 'selecting' && (
                <ContainerSelector
                    containers={containers}
                    podName={pod}
                    title="Select Container"
                    description={<>Choose a container to browse files in <span className="font-medium text-foreground">{pod}</span></>}
                    onSelect={handleContainerSelect}
                />
            )}

            {/* Stale Tab Banner */}
            {state === 'browsing' && isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-red-900/30 border-b border-red-500/50 text-red-400 shrink-0">
                    <ExclamationTriangleIcon className="h-5 w-5" />
                    <span className="text-sm">
                        Read-only: This file browser is from context <span className="font-medium">{tabContext}</span>.
                    </span>
                </div>
            )}

            {/* Header - only show when browsing */}
            {state === 'browsing' && (
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    {/* Container selector */}
                    {containers.length > 1 && (
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-500">Container:</span>
                            <div className="w-40">
                                <SearchSelect
                                    options={containers}
                                    value={selectedContainer}
                                    onChange={setSelectedContainer}
                                    placeholder="Select..."
                                    className="text-xs"
                                    getOptionValue={(c: any) => typeof c === 'object' ? c.name : c}
                                    getOptionLabel={(c: any) => typeof c === 'object' ? c.name : c}
                                />
                            </div>
                        </div>
                    )}

                    {/* Breadcrumb */}
                    <div className="flex items-center gap-1 text-sm">
                        <button
                            onClick={() => navigateTo('/')}
                            className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded"
                        >
                            <HomeIcon className="h-4 w-4" />
                        </button>
                        {pathParts.map((part: any, idx: number) => (
                            <React.Fragment key={idx}>
                                <ChevronRightIcon className="h-3 w-3 text-gray-600" />
                                <button
                                    onClick={() => navigateTo('/' + pathParts.slice(0, idx + 1).join('/'))}
                                    className="px-1 text-gray-400 hover:text-white hover:bg-white/10 rounded truncate max-w-[120px]"
                                    title={part}
                                >
                                    {part}
                                </button>
                            </React.Fragment>
                        ))}
                    </div>

                    {/* Selection count */}
                    {selectedFiles.size > 0 && (
                        <div className="flex items-center gap-2 text-xs text-gray-400">
                            <span className="px-2 py-0.5 bg-primary/20 text-primary rounded">
                                {selectedFiles.size} selected
                            </span>
                            <button
                                onClick={() => setSelectedFiles(new Set<any>())}
                                className="text-gray-500 hover:text-white"
                            >
                                Clear
                            </button>
                        </div>
                    )}
                </div>

                {/* Actions */}
                <div className="flex items-center gap-1">
                    <Tooltip content="Refresh">
                        <button
                            onClick={loadFiles}
                            disabled={loading || isStale}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 disabled:opacity-50"
                        >
                            <ArrowPathIcon className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
                        </button>
                    </Tooltip>

                    <div className="w-px h-4 bg-border mx-1" />

                    <Tooltip content="Upload file">
                        <button
                            onClick={handleUpload}
                            disabled={isStale || !!progress}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 disabled:opacity-50"
                        >
                            <ArrowUpTrayIcon className="h-4 w-4" />
                        </button>
                    </Tooltip>

                    <Tooltip content="New folder">
                        <button
                            onClick={() => setShowNewFolderInput(true)}
                            disabled={isStale}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 disabled:opacity-50"
                        >
                            <FolderPlusIcon className="h-4 w-4" />
                        </button>
                    </Tooltip>

                    <div className="w-px h-4 bg-border mx-1" />

                    <Tooltip content={selectedFiles.size > 0 ? `Download ${selectedFiles.size} item${selectedFiles.size > 1 ? 's' : ''}` : "Download"}>
                        <button
                            onClick={handleDownload}
                            disabled={selectedFiles.size === 0 || isStale || !!progress}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 disabled:opacity-50"
                        >
                            <ArrowDownTrayIcon className="h-4 w-4" />
                        </button>
                    </Tooltip>

                    <Tooltip content={selectedFiles.size > 0 ? `Delete ${selectedFiles.size} item${selectedFiles.size > 1 ? 's' : ''}` : "Delete"}>
                        <button
                            onClick={handleDelete}
                            disabled={selectedFiles.size === 0 || isStale}
                            className="p-1.5 rounded text-gray-400 hover:text-red-400 hover:bg-red-500/10 disabled:opacity-50"
                        >
                            <TrashIcon className="h-4 w-4" />
                        </button>
                    </Tooltip>
                </div>
            </div>
            )}

            {/* New folder input */}
            {state === 'browsing' && showNewFolderInput && (
                <div className="flex items-center gap-2 px-4 py-2 border-b border-border bg-surface/50">
                    <FolderPlusIcon className="h-4 w-4 text-gray-500" />
                    <input
                        ref={newFolderInputRef}
                        type="text"
                        placeholder="New folder name..."
                        value={newFolderName}
                        onChange={(e: any) => setNewFolderName(e.target.value)}
                        onKeyDown={(e) => {
                            if (e.key === 'Enter') handleCreateFolder();
                            if (e.key === 'Escape') {
                                setShowNewFolderInput(false);
                                setNewFolderName('');
                            }
                        }}
                        className="flex-1 bg-background border border-border rounded px-2 py-1 text-sm text-white focus:outline-none focus:border-primary"
                    />
                    <button
                        onClick={handleCreateFolder}
                        disabled={!newFolderName.trim()}
                        className="px-3 py-1 text-sm bg-primary text-white rounded hover:bg-primary/80 disabled:opacity-50"
                    >
                        Create
                    </button>
                    <button
                        onClick={() => {
                            setShowNewFolderInput(false);
                            setNewFolderName('');
                        }}
                        className="px-3 py-1 text-sm text-gray-400 hover:text-white"
                    >
                        Cancel
                    </button>
                </div>
            )}

            {/* File list */}
            {state === 'browsing' && (
            <div className="flex-1 overflow-auto">
                {loading ? (
                    <div className="flex items-center justify-center h-full">
                        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary" />
                    </div>
                ) : error ? (
                    <div className="flex flex-col items-center justify-center h-full text-red-400">
                        <ExclamationTriangleIcon className="h-8 w-8 mb-2" />
                        <span className="text-sm">{error}</span>
                        <button
                            onClick={loadFiles}
                            className="mt-3 px-4 py-2 text-sm bg-primary/20 text-primary rounded hover:bg-primary/30"
                        >
                            Retry
                        </button>
                    </div>
                ) : files.length === 0 ? (
                    <div className="flex items-center justify-center h-full text-gray-500">
                        <span>Empty directory</span>
                    </div>
                ) : (
                    <table className="w-full text-sm">
                        <thead className="sticky top-0 bg-surface border-b border-border">
                            <tr className="text-left text-xs text-gray-500">
                                <th className="pl-4 pr-2 py-2 w-8">
                                    <input
                                        type="checkbox"
                                        checked={selectableFiles.length > 0 && selectedFiles.size === selectableFiles.length}
                                        onChange={toggleSelectAll}
                                        className="w-3.5 h-3.5 rounded border-gray-500 bg-transparent text-primary focus:ring-primary focus:ring-offset-0 cursor-pointer"
                                    />
                                </th>
                                <th className="px-2 py-2 font-medium">Name</th>
                                <th className="px-4 py-2 font-medium w-24 text-right">Size</th>
                                <th className="px-4 py-2 font-medium w-28">Permissions</th>
                                <th className="px-4 py-2 font-medium w-20">Owner</th>
                                <th className="px-4 py-2 font-medium w-36">Modified</th>
                            </tr>
                        </thead>
                        <tbody>
                            {files.map((file: any, idx: number) => (
                                <tr
                                    key={idx}
                                    onClick={() => toggleFileSelection(file)}
                                    onDoubleClick={() => handleFileDoubleClick(file)}
                                    className={`cursor-pointer transition-colors ${
                                        selectedFiles.has(file.name)
                                            ? 'bg-primary/20'
                                            : 'hover:bg-white/5'
                                    }`}
                                >
                                    <td className="pl-4 pr-2 py-2">
                                        {file.name !== '..' ? (
                                            <input
                                                type="checkbox"
                                                checked={selectedFiles.has(file.name)}
                                                onChange={(e: any) => toggleFileSelection(file, e)}
                                                onClick={(e) => e.stopPropagation()}
                                                className="w-3.5 h-3.5 rounded border-gray-500 bg-transparent text-primary focus:ring-primary focus:ring-offset-0 cursor-pointer"
                                            />
                                        ) : (
                                            <span className="inline-block w-3.5 h-3.5" />
                                        )}
                                    </td>
                                    <td className="px-2 py-2">
                                        <div className="flex items-center gap-2">
                                            {file.isDir ? (
                                                <FolderIcon className="h-4 w-4 text-amber-400 shrink-0" />
                                            ) : (
                                                <DocumentIcon className="h-4 w-4 text-gray-400 shrink-0" />
                                            )}
                                            <span className={`truncate ${file.isDir ? 'text-white' : 'text-gray-300'}`}>
                                                {file.name}
                                            </span>
                                        </div>
                                    </td>
                                    <td className="px-4 py-2 text-right text-gray-400 font-mono text-xs">
                                        {file.isDir ? '-' : formatBytes(file.size)}
                                    </td>
                                    <td className="px-4 py-2 text-gray-500 font-mono text-xs">
                                        {file.permissions}
                                    </td>
                                    <td className="px-4 py-2 text-gray-500 text-xs">
                                        {file.owner}
                                    </td>
                                    <td className="px-4 py-2 text-gray-500 text-xs">
                                        {file.modTime}
                                    </td>
                                </tr>
                            ))}
                        </tbody>
                    </table>
                )}
            </div>
            )}

            {/* Drag overlay - only show when browsing */}
            {state === 'browsing' && (
            <div
                className={`absolute inset-0 bg-primary/10 border-2 border-dashed border-primary rounded-lg flex items-center justify-center z-10 pointer-events-none transition-opacity duration-150 ${
                    isDragging && !isStale ? 'opacity-100' : 'opacity-0 invisible'
                }`}
            >
                <div className="text-center">
                    <ArrowUpTrayIcon className="h-12 w-12 text-primary mx-auto mb-2" />
                    <span className="text-primary font-medium">Drop files to upload to {currentPath}</span>
                </div>
            </div>
            )}

            {/* Progress bar */}
            {state === 'browsing' && <ProgressBar progress={progress} />}
        </div>
    );
}
