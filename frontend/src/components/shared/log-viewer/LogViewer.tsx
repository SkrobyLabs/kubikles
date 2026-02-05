import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Virtuoso } from 'react-virtuoso';
import { GetAllPodLogs, SavePodLogs, SaveLogsBundle } from '../../../../wailsjs/go/main/App';
import { useK8s, useConfig } from '../../../context';
import {
    ArrowDownTrayIcon,
    ArchiveBoxArrowDownIcon,
    ClockIcon,
    Bars3BottomLeftIcon,
    BackwardIcon,
    ChevronDoubleUpIcon,
    ChevronDoubleDownIcon,
    CalendarIcon,
    ExclamationTriangleIcon,
    ArrowPathIcon,
    EyeIcon,
    MagnifyingGlassIcon,
    XMarkIcon,
    FunnelIcon,
    MinusIcon,
    PlusIcon,
    ArrowsPointingOutIcon,
    BugAntIcon
} from '@heroicons/react/24/outline';

import SearchSelect from '../SearchSelect';
import Tooltip from '../Tooltip';

import { useLogStream, ALL_CONTAINERS, ALL_PODS } from './useLogStream';
import { useLogSearch } from './useLogSearch';
import { LogLine, Spinner } from './LogLine';
import { TimePickerModal } from './TimePickerModal';
import { logsToVisibleString, logsToDebugString } from './logUtils';
import { GetAllContainersLogsAll, GetAllPodsLogsAll } from '../../../../wailsjs/go/main/App';

export default function LogViewer({
    namespace,
    pod,
    containers = [],
    siblingPods = [],
    podContainerMap = {},
    ownerName = '',
    podCreationTime = '',
    tabContext = ''
}) {
    const { currentContext } = useK8s();
    const { getConfig } = useConfig();

    // Helper to safely get config with validation and fallback
    const getSafeConfig = useCallback((path, defaultValue, validator) => {
        try {
            const value = getConfig(path);
            if (value === undefined || value === null) return defaultValue;
            if (validator && !validator(value)) {
                console.error(`Invalid config value for ${path}:`, value, '- using default:', defaultValue);
                return defaultValue;
            }
            return value;
        } catch (e) {
            console.error(`Error reading config ${path}:`, e, '- using default:', defaultValue);
            return defaultValue;
        }
    }, [getConfig]);

    // UI state
    // Always default to the specific pod that was requested
    const [selectedPod, setSelectedPod] = useState(pod);
    // Default to first container for the selected pod
    const [selectedContainer, setSelectedContainer] = useState(containers[0] || '');
    const [wrapLines, setWrapLines] = useState(() => getSafeConfig('logs.lineWrap', true, v => typeof v === 'boolean'));
    const [showTimestamps, setShowTimestamps] = useState(() => getSafeConfig('logs.showTimestamps', false, v => typeof v === 'boolean'));
    const [showPrevious, setShowPrevious] = useState(false);
    const [showTimeModal, setShowTimeModal] = useState(false);
    const [sinceTime, setSinceTime] = useState('');
    const initialPosition = getSafeConfig('logs.position', 'end', v => ['start', 'end', 'all'].includes(v));
    const [viewMode, setViewMode] = useState(initialPosition === 'all' ? 'start' : initialPosition);
    const [autoFollow, setAutoFollow] = useState(true);
    const [downloading, setDownloading] = useState(false);
    const [downloadingBundle, setDownloadingBundle] = useState(false);

    const showDebugDownload = getConfig('debug.showLogSourceMarkers');

    const virtuosoRef = useRef(null);
    const isAtBottomRef = useRef(true);

    // Check if this tab is stale
    const isStale = tabContext && tabContext !== currentContext;

    // Get current containers for the selected pod (or first pod if "All Pods")
    const currentContainers = selectedPod === ALL_PODS
        ? (podContainerMap[siblingPods[0]] || containers)
        : (podContainerMap[selectedPod] || containers);

    // Log streaming hook
    const stream = useLogStream({
        namespace,
        pod: selectedPod,
        container: selectedContainer,
        containers: currentContainers, // Pass all containers for "All Containers" mode
        siblingPods, // Pass sibling pods for "All Pods" mode
        podContainerMap, // Pass pod-container map for "All Pods" mode
        showPrevious,
        sinceTime,
        viewMode,
        initialPosition,
        isStale,
        currentContext
    });

    // Search hook
    const search = useLogSearch({
        logs: stream.logs,
        getConfig,
        getSafeConfig
    });

    // Auto-follow is active when in 'end' mode with no custom time filter
    const isFollowing = viewMode === 'end' && !sinceTime && !showPrevious && autoFollow && !isStale && !stream.isAllLoaded;

    // Manage streaming based on follow state
    useEffect(() => {
        // Use isFetching() for synchronous check to avoid React batching race condition
        // where stream.loading might still be false when this effect runs
        if (isFollowing && namespace && selectedPod && !stream.loading && !stream.isFetching()) {
            stream.startStreaming();
        } else {
            stream.stopStreaming();
        }
        return () => stream.stopStreaming();
    }, [isFollowing, namespace, selectedPod, selectedContainer, stream.loading]);

    // Keyboard shortcut for refresh
    useEffect(() => {
        const handleKeyDown = (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'r') {
                if (namespace && selectedPod) {
                    stream.fetchLogs();
                }
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [namespace, selectedPod, selectedContainer, showPrevious, sinceTime, viewMode, stream.fetchLogs]);

    // Virtuoso callbacks
    const handleAtBottomStateChange = useCallback((atBottom) => {
        isAtBottomRef.current = atBottom;
        if (!atBottom && !stream.hasMoreAfter && !isFollowing && !stream.isAllLoaded) {
            stream.setHasMoreAfter(true);
        }
    }, [stream.hasMoreAfter, isFollowing, stream.isAllLoaded]);

    const handleStartReached = useCallback(() => {
        if (stream.isAllLoaded || !stream.hasMoreBefore) return;
        stream.loadOlderLogs();
    }, [stream.isAllLoaded, stream.hasMoreBefore, stream.loadOlderLogs]);

    const handleEndReached = useCallback(() => {
        if (stream.isAllLoaded || !stream.hasMoreAfter || isFollowing) return;
        stream.loadNewerLogs();
    }, [stream.isAllLoaded, stream.hasMoreAfter, isFollowing, stream.loadNewerLogs]);

    const followOutput = useCallback((isAtBottom) => {
        if (isFollowing && isAtBottom) return 'smooth';
        return false;
    }, [isFollowing]);

    // Scroll helpers
    const scrollToTop = useCallback(() => {
        isAtBottomRef.current = false;
        virtuosoRef.current?.scrollToIndex({ index: 0, behavior: 'auto' });
    }, []);

    const scrollToBottom = useCallback(() => {
        isAtBottomRef.current = true;
        virtuosoRef.current?.scrollToIndex({ index: 'LAST', behavior: 'auto' });
    }, []);

    // Navigation
    const jumpToStart = () => {
        stream.resetChunkState();
        stream.setLogs([]);
        stream.setHasMoreBefore(false);
        stream.setHasMoreAfter(true);
        stream.setIsAllLoaded(false);
        setViewMode('start');
        setSinceTime('');
    };

    const jumpToEnd = () => {
        if (viewMode === 'end' && !sinceTime && !showPrevious && !stream.isAllLoaded) {
            const newAutoFollow = !autoFollow;
            setAutoFollow(newAutoFollow);
            if (newAutoFollow) {
                scrollToBottom();
                stream.setHasMoreAfter(false);
            } else {
                stream.setHasMoreAfter(true);
            }
            return;
        }

        stream.resetChunkState();
        stream.setLogs([]);
        stream.setHasMoreBefore(true);
        stream.setHasMoreAfter(false);
        stream.setIsAllLoaded(false);
        setViewMode('end');
        setSinceTime('');
        setAutoFollow(true);
        scrollToBottom();
    };

    const handleTimeApply = (time) => {
        setSinceTime(time);
        setViewMode('end');
        setShowTimeModal(false);
    };

    const handleLoadAll = async () => {
        await stream.loadAllLogs();
        setViewMode('start');
        requestAnimationFrame(() => scrollToTop());
    };

    // Downloads
    const downloadLogs = async () => {
        setDownloading(true);
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const podSuffix = selectedPod === ALL_PODS ? 'all-pods' : selectedPod;
        const containerSuffix = selectedContainer === ALL_CONTAINERS ? 'all-containers' : selectedContainer;
        const filename = `${podSuffix}-${containerSuffix}-${timestamp}.log`;
        try {
            let allLogs;
            const isAllContainers = selectedContainer === ALL_CONTAINERS;

            if (selectedPod === ALL_PODS) {
                // Build pod-container pairs for all pods
                const podPairs = siblingPods.map(podName => ({
                    podName,
                    containerNames: isAllContainers
                        ? (podContainerMap[podName] || containers)
                        : (selectedContainer ? [selectedContainer] : [])
                }));
                allLogs = await GetAllPodsLogsAll(namespace, podPairs, isAllContainers, showTimestamps, showPrevious);
            } else if (isAllContainers) {
                allLogs = await GetAllContainersLogsAll(namespace, selectedPod, currentContainers, showTimestamps, showPrevious);
            } else {
                allLogs = await GetAllPodLogs(namespace, selectedPod, selectedContainer, showTimestamps, showPrevious);
            }
            await SavePodLogs(allLogs, filename);
        } catch (err) {
            console.error('Failed to save logs:', err);
        } finally {
            setDownloading(false);
        }
    };

    const downloadBundle = async () => {
        setDownloadingBundle(true);
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const bundleName = ownerName || pod;
        const filename = `${bundleName}-${timestamp}.zip`;

        try {
            const entries = [];
            const podsToDownload = siblingPods.length > 0 ? siblingPods : [pod];

            for (const podName of podsToDownload) {
                const podContainers = podContainerMap[podName] || containers;
                for (const containerName of podContainers) {
                    try {
                        const logs = await GetAllPodLogs(namespace, podName, containerName, showTimestamps, showPrevious);
                        entries.push({ podName, containerName, logs: logs || '' });
                    } catch (err) {
                        entries.push({ podName, containerName, logs: `Error fetching logs: ${err}` });
                    }
                }
            }
            await SaveLogsBundle(entries, filename);
        } catch (err) {
            console.error('Failed to save logs bundle:', err);
        } finally {
            setDownloadingBundle(false);
        }
    };

    const downloadVisibleLogs = async () => {
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `${selectedPod}-${timestamp}.log`;
        try {
            await SavePodLogs(logsToVisibleString(stream.logs, showTimestamps), filename);
        } catch (err) {
            console.error('Failed to save visible logs:', err);
        }
    };

    const downloadDebugLogs = async () => {
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `DEBUG-${selectedPod}-${timestamp}.log`;
        try {
            await SavePodLogs(logsToDebugString(stream.logs), filename);
        } catch (err) {
            console.error('Failed to save debug logs:', err);
        }
    };

    // Render log item
    const renderLogItem = useCallback((index) => {
        const entry = search.displayLogs[index];
        if (!entry) return null;

        return (
            <LogLine
                entry={entry}
                showTimestamps={showTimestamps}
                searchTerm={search.searchTerm}
                searchRegex={search.searchRegex}
                wrapLines={wrapLines}
            />
        );
    }, [search.displayLogs, showTimestamps, search.searchTerm, search.searchRegex, wrapLines]);

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <ExclamationTriangleIcon className="h-5 w-5" />
                    <span className="text-sm">
                        Read-only: These logs are from context <span className="font-medium">{tabContext}</span>. Switch back to view live logs.
                    </span>
                </div>
            )}

            {/* Stream Disconnected Banner */}
            {stream.streamDisconnected && !isStale && (
                <div className="flex items-center justify-between px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <div className="flex items-center gap-2">
                        <ExclamationTriangleIcon className="h-5 w-5" />
                        <span className="text-sm">{stream.disconnectReason || 'Stream disconnected'}</span>
                    </div>
                    <button
                        onClick={stream.fetchLogs}
                        disabled={stream.loading}
                        className="flex items-center gap-1.5 px-3 py-1 text-xs font-medium bg-amber-600 text-white rounded hover:bg-amber-500 transition-colors disabled:opacity-50"
                    >
                        <ArrowPathIcon className={`h-4 w-4 ${stream.loading ? 'animate-spin' : ''}`} />
                        Refresh
                    </button>
                </div>
            )}

            {/* Fetch Error Banner */}
            {stream.fetchError && (
                <div className="flex items-center justify-between px-4 py-1.5 bg-amber-900/20 border-b border-amber-500/30 text-amber-400 shrink-0">
                    <div className="flex items-center gap-2">
                        <ExclamationTriangleIcon className="h-4 w-4" />
                        <span className="text-xs">{stream.fetchError}</span>
                    </div>
                    <button
                        onClick={stream.clearFetchError}
                        className="text-xs text-amber-400 hover:text-amber-300 px-2"
                    >
                        Dismiss
                    </button>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-500">Pod:</span>
                        <div className="w-64">
                            <SearchSelect
                                options={siblingPods.length > 1
                                    ? [ALL_PODS, ...siblingPods]
                                    : (siblingPods.length > 0 ? siblingPods : [pod])
                                }
                                value={selectedPod}
                                onChange={setSelectedPod}
                                placeholder="Select Pod..."
                                className="text-xs"
                                getOptionLabel={(opt) => opt === ALL_PODS ? 'All Pods' : opt}
                            />
                        </div>
                    </div>
                    {currentContainers.length > 0 && (
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-500">Container:</span>
                            <div className="w-48">
                                <SearchSelect
                                    options={currentContainers.length > 1
                                        ? [ALL_CONTAINERS, ...currentContainers]
                                        : currentContainers
                                    }
                                    value={selectedContainer}
                                    onChange={setSelectedContainer}
                                    placeholder="Select Container..."
                                    className="text-xs"
                                    getOptionLabel={(opt) => opt === ALL_CONTAINERS ? 'All Containers' : opt}
                                    disabled={!!stream.fetchError || stream.streamDisconnected}
                                />
                            </div>
                        </div>
                    )}
                </div>
                <div className="flex items-center gap-1">
                    {/* Search Toggle */}
                    <Tooltip content={search.showSearch ? 'Close search' : 'Search (⌘F)'}>
                        <button
                            onClick={() => search.showSearch ? search.closeSearch() : search.openSearch()}
                            className={`p-1.5 rounded transition-colors ${search.showSearch ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <MagnifyingGlassIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    <div className="w-px h-4 bg-border mx-1" />

                    {/* Wrap Lines Toggle */}
                    <Tooltip content={wrapLines ? 'Disable line wrap' : 'Enable line wrap'}>
                        <button
                            onClick={() => setWrapLines(!wrapLines)}
                            className={`p-1.5 rounded transition-colors ${wrapLines ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <Bars3BottomLeftIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    {/* Timestamps Toggle */}
                    <Tooltip content={showTimestamps ? 'Hide timestamps' : 'Show timestamps'}>
                        <button
                            onClick={() => setShowTimestamps(!showTimestamps)}
                            className={`p-1.5 rounded transition-colors ${showTimestamps ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <ClockIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    {/* Previous Container Logs Toggle */}
                    <Tooltip content={showPrevious ? 'Showing previous container logs' : 'Show previous container logs'}>
                        <button
                            onClick={() => setShowPrevious(!showPrevious)}
                            disabled={!!stream.fetchError || stream.streamDisconnected}
                            className={`p-1.5 rounded transition-colors disabled:opacity-50 ${showPrevious ? 'bg-amber-500/20 text-amber-400' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <BackwardIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    <div className="w-px h-4 bg-border mx-1" />

                    {/* Jump to Start */}
                    <Tooltip content="Jump to start">
                        <button
                            onClick={jumpToStart}
                            disabled={stream.loading || stream.loadingAll || !!stream.fetchError || stream.streamDisconnected}
                            className={`p-1.5 rounded transition-colors disabled:opacity-50 ${viewMode === 'start' && !stream.isAllLoaded ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <ChevronDoubleUpIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    {/* Jump to End / Follow */}
                    <Tooltip content={isFollowing ? "Following logs (click to pause)" : viewMode === 'end' && !sinceTime && !showPrevious ? "Click to resume following" : "Jump to end & follow"}>
                        <button
                            onClick={jumpToEnd}
                            disabled={stream.loading || stream.loadingAll || !!stream.fetchError || stream.streamDisconnected}
                            className={`p-1.5 rounded transition-colors disabled:opacity-50 ${isFollowing ? 'bg-green-500/20 text-green-400' : viewMode === 'end' && !sinceTime && !stream.isAllLoaded ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <ChevronDoubleDownIcon className={`w-4 h-4 ${isFollowing ? 'animate-pulse' : ''}`} />
                        </button>
                    </Tooltip>

                    {/* Jump to Time */}
                    <Tooltip content={sinceTime ? `Filtering from: ${sinceTime}` : 'Jump to time'}>
                        <button
                            onClick={() => setShowTimeModal(true)}
                            disabled={stream.loadingAll || !!stream.fetchError || stream.streamDisconnected}
                            className={`p-1.5 rounded transition-colors disabled:opacity-50 ${sinceTime ? 'bg-green-500/20 text-green-400' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <CalendarIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    {/* Load All Logs */}
                    <Tooltip content={stream.isAllLoaded ? "All logs loaded" : "Load all logs"}>
                        <button
                            onClick={handleLoadAll}
                            disabled={stream.loading || stream.loadingAll || stream.isAllLoaded || !!stream.fetchError || stream.streamDisconnected}
                            className={`p-1.5 rounded transition-colors disabled:opacity-50 ${stream.isAllLoaded ? 'bg-green-500/20 text-green-400' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            {stream.loadingAll ? <Spinner /> : <ArrowsPointingOutIcon className="w-4 h-4" />}
                        </button>
                    </Tooltip>

                    <div className="w-px h-4 bg-border mx-1" />

                    {/* Download container logs */}
                    <Tooltip content={
                        selectedPod === ALL_PODS
                            ? (selectedContainer === ALL_CONTAINERS ? "Download merged logs (all pods, all containers)" : "Download merged logs (all pods)")
                            : (selectedContainer === ALL_CONTAINERS ? "Download merged logs (all containers)" : "Download container logs")
                    }>
                        <button
                            onClick={downloadLogs}
                            disabled={stream.logs.length === 0 || stream.loading || downloading || !!stream.fetchError || stream.streamDisconnected}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            {downloading ? <Spinner /> : <ArrowDownTrayIcon className="w-4 h-4" />}
                        </button>
                    </Tooltip>

                    {/* Download all pod logs */}
                    {siblingPods.length > 1 && (
                        <Tooltip content="Download all pod logs (zip)">
                            <button
                                onClick={downloadBundle}
                                disabled={stream.loading || downloadingBundle || !!stream.fetchError || stream.streamDisconnected}
                                className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                {downloadingBundle ? <Spinner /> : <ArchiveBoxArrowDownIcon className="w-4 h-4" />}
                            </button>
                        </Tooltip>
                    )}

                    {/* Download visible logs */}
                    <Tooltip content="Download visible logs">
                        <button
                            onClick={downloadVisibleLogs}
                            disabled={stream.logs.length === 0}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        >
                            <EyeIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    {/* Debug download (with source markers) - enable via debug.showLogSourceMarkers config */}
                    {showDebugDownload && (
                        <Tooltip content="Download with source markers (debug)">
                            <button
                                onClick={downloadDebugLogs}
                                disabled={stream.logs.length === 0}
                                className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            >
                                <BugAntIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                    )}
                </div>
            </div>

            {/* Search Bar */}
            {search.showSearch && (
                <div className="flex items-center gap-2 px-4 py-2 border-b border-border bg-surface shrink-0">
                    <div className="relative flex-1 max-w-md">
                        <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-3 top-1/2 transform -translate-y-1/2" />
                        <input
                            ref={search.searchInputRef}
                            type="text"
                            placeholder={search.isRegex ? (search.searchOnEnter ? "Search (regex, Enter)..." : "Search (regex)...") : (search.searchOnEnter ? "Search (Enter)..." : "Search...")}
                            className={`w-full bg-background border rounded-md pl-9 pr-8 py-1.5 text-sm text-white focus:outline-none transition-colors ${search.regexError ? 'border-red-500' : 'border-border focus:border-primary'}`}
                            value={search.searchInput}
                            onChange={(e) => search.setSearchInput(e.target.value)}
                            onKeyDown={search.handleSearchKeyDown}
                            autoComplete="off"
                            autoCorrect="off"
                            spellCheck="false"
                        />
                        {search.searchInput && (
                            <button
                                onClick={search.clearSearchInput}
                                className="absolute right-2 top-1/2 transform -translate-y-1/2 text-gray-400 hover:text-white"
                            >
                                <XMarkIcon className="h-4 w-4" />
                            </button>
                        )}
                    </div>

                    {/* Match count */}
                    {search.searchTerm && !search.regexError && (
                        <span className="text-xs text-gray-400 min-w-[60px]">
                            {search.matchCount} match{search.matchCount !== 1 ? 'es' : ''}
                        </span>
                    )}
                    {search.regexError && (
                        <Tooltip content={search.regexError}>
                            <span className="text-xs text-red-400 truncate max-w-[150px]">Invalid regex</span>
                        </Tooltip>
                    )}

                    <div className="w-px h-4 bg-border" />

                    {/* Search on Enter Toggle */}
                    <Tooltip content={search.searchOnEnter ? 'Search on Enter (click for as-you-type)' : 'Search as you type (click for Enter mode)'}>
                        <button
                            onClick={() => search.setSearchOnEnter(!search.searchOnEnter)}
                            className={`px-2 py-1 text-xs rounded transition-colors font-mono ${search.searchOnEnter ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            ↵
                        </button>
                    </Tooltip>

                    {/* Regex Toggle */}
                    <Tooltip content={search.isRegex ? 'Using regex' : 'Use regex'}>
                        <button
                            onClick={() => search.setIsRegex(!search.isRegex)}
                            className={`px-2 py-1 text-xs rounded transition-colors ${search.isRegex ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            .*
                        </button>
                    </Tooltip>

                    {/* Filter Only Toggle */}
                    <Tooltip content={search.filterOnly ? 'Showing only matching lines' : 'Show only matching lines'}>
                        <button
                            onClick={() => search.setFilterOnly(!search.filterOnly)}
                            className={`p-1.5 rounded transition-colors ${search.filterOnly ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        >
                            <FunnelIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>

                    {/* Context Lines (only when filter is active) */}
                    {search.filterOnly && (
                        <>
                            <div className="w-px h-4 bg-border" />
                            <div className="flex items-center gap-1 text-xs text-gray-400">
                                <span>±</span>
                                <Tooltip content="Decrease context lines before">
                                    <button
                                        onClick={() => search.setContextLinesBefore(Math.max(0, search.contextLinesBefore - 1))}
                                        className="p-0.5 rounded hover:bg-white/10 disabled:opacity-30"
                                        disabled={search.contextLinesBefore === 0}
                                    >
                                        <MinusIcon className="w-3 h-3" />
                                    </button>
                                </Tooltip>
                                <span className="w-4 text-center">{search.contextLinesBefore}</span>
                                <Tooltip content="Increase context lines before">
                                    <button
                                        onClick={() => search.setContextLinesBefore(search.contextLinesBefore + 1)}
                                        className="p-0.5 rounded hover:bg-white/10"
                                    >
                                        <PlusIcon className="w-3 h-3" />
                                    </button>
                                </Tooltip>
                                <span className="text-gray-600">/</span>
                                <Tooltip content="Decrease context lines after">
                                    <button
                                        onClick={() => search.setContextLinesAfter(Math.max(0, search.contextLinesAfter - 1))}
                                        className="p-0.5 rounded hover:bg-white/10 disabled:opacity-30"
                                        disabled={search.contextLinesAfter === 0}
                                    >
                                        <MinusIcon className="w-3 h-3" />
                                    </button>
                                </Tooltip>
                                <span className="w-4 text-center">{search.contextLinesAfter}</span>
                                <Tooltip content="Increase context lines after">
                                    <button
                                        onClick={() => search.setContextLinesAfter(search.contextLinesAfter + 1)}
                                        className="p-0.5 rounded hover:bg-white/10"
                                    >
                                        <PlusIcon className="w-3 h-3" />
                                    </button>
                                </Tooltip>
                            </div>
                        </>
                    )}

                    <div className="w-px h-4 bg-border" />

                    {/* Close Search */}
                    <Tooltip content="Close search (Esc)">
                        <button
                            onClick={search.closeSearch}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                        >
                            <XMarkIcon className="w-4 h-4" />
                        </button>
                    </Tooltip>
                </div>
            )}

            {/* Logs Content */}
            <div className="flex-1 overflow-hidden text-gray-300 font-mono text-xs">
                {stream.loading || stream.loadingAll ? (
                    <div className="flex flex-col items-center justify-center h-full">
                        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                        {stream.loadingAll && (
                            <span className="mt-3 text-gray-500 text-sm">Loading all logs...</span>
                        )}
                    </div>
                ) : stream.logs.length === 0 ? (
                    <div className="flex items-center justify-center h-full">
                        {showPrevious ? (
                            <span className="text-amber-400">No previous container logs available.</span>
                        ) : (
                            <span className="text-gray-500">No logs available.</span>
                        )}
                    </div>
                ) : search.searchTerm && search.matchCount === 0 && !stream.isAllLoaded ? (
                    <div className="flex flex-col items-center justify-center h-full text-gray-400">
                        <span className="mb-3">No matches found in loaded logs</span>
                        <button
                            onClick={handleLoadAll}
                            className="px-4 py-2 text-sm bg-primary/20 text-primary rounded hover:bg-primary/30 transition-colors"
                        >
                            Load all logs and search
                        </button>
                    </div>
                ) : (
                    <Virtuoso
                        ref={virtuosoRef}
                        style={{ height: '100%' }}
                        className="px-4"
                        data={search.displayLogs}
                        firstItemIndex={stream.firstItemIndex}
                        initialTopMostItemIndex={viewMode === 'start' ? 0 : search.displayLogs.length - 1}
                        itemContent={(index) => renderLogItem(index - stream.firstItemIndex)}
                        followOutput={followOutput}
                        atBottomStateChange={handleAtBottomStateChange}
                        startReached={handleStartReached}
                        endReached={handleEndReached}
                        overscan={200}
                        components={{
                            Header: () => (
                                <>
                                    {stream.loadingBefore && (
                                        <div className="flex items-center justify-center py-2 text-gray-500">
                                            <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary mr-2"></div>
                                            Loading older logs...
                                        </div>
                                    )}
                                    {!stream.hasMoreBefore && !stream.loadingBefore && (
                                        <div className="flex items-center justify-center py-1 text-gray-600 text-xs">
                                            — You are at the top —
                                        </div>
                                    )}
                                </>
                            ),
                            Footer: () => (
                                stream.loadingAfter ? (
                                    <div className="flex items-center justify-center py-2 text-gray-500">
                                        <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary mr-2"></div>
                                        Loading newer logs...
                                    </div>
                                ) : null
                            )
                        }}
                    />
                )}
            </div>

            {/* Time Picker Modal */}
            <TimePickerModal
                show={showTimeModal}
                onClose={() => setShowTimeModal(false)}
                onApply={handleTimeApply}
                sinceTime={sinceTime}
                getFirstTimestamp={stream.getFirstTimestamp}
                podCreationTime={podCreationTime}
            />

        </div>
    );
}
