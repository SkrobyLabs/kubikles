import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { GetPodLogs, GetAllPodLogs, GetPodLogsFromStart, GetPodLogsBefore, GetPodLogsAfter, SavePodLogs, SaveLogsBundle, StartLogStream, StopLogStream } from '../../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';
import { useK8s } from '../../context/K8sContext';
import { useDebug } from '../../context/DebugContext';
import Convert from 'ansi-to-html';
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
    BugAntIcon,
    EyeIcon
} from '@heroicons/react/24/outline';

const converter = new Convert({
    fg: '#FFF',
    bg: '#1e1e1e',
    newline: true,
    escapeXML: true
});

// Fix non-standard 4-digit 256-color codes (e.g., 0008 -> 8) to standard format
const normalizeAnsiCodes = (text) => {
    return text.replace(/\x1b\[38;5;0*(\d{1,3})m/g, '\x1b[38;5;$1m')
               .replace(/\x1b\[48;5;0*(\d{1,3})m/g, '\x1b[48;5;$1m');
};

import SearchSelect from './SearchSelect';

// Spinner component for loading states
const Spinner = () => (
    <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
        <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
        <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
    </svg>
);

// Validate RFC3339 datetime format (e.g., 2024-11-26T14:30:00Z)
const isValidDateTime = (str) => {
    if (!str) return false;
    // Accept formats like: 2024-11-26T14:30:00Z, 2024-11-26T14:30:00, 2024-11-26 14:30:00
    const patterns = [
        /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z?$/,
        /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/,
        /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/,
        /^\d{4}-\d{2}-\d{2} \d{2}:\d{2}$/
    ];
    if (!patterns.some(p => p.test(str))) return false;
    const date = new Date(str.replace(' ', 'T'));
    return !isNaN(date.getTime());
};

// Convert input to RFC3339 format
const toRFC3339 = (str) => {
    if (!str) return '';
    let normalized = str.replace(' ', 'T');
    if (!normalized.includes(':00Z') && !normalized.endsWith('Z')) {
        if (normalized.length === 16) { // 2024-11-26T14:30
            normalized += ':00Z';
        } else if (normalized.length === 19) { // 2024-11-26T14:30:00
            normalized += 'Z';
        }
    }
    return normalized;
};

// Log entry structure: { timestamp: string, content: string, source: 'initial'|'before'|'after'|'stream' }

// Parse a raw log string (with timestamps) into structured log entries
const parseLogLines = (rawLogs, source) => {
    if (!rawLogs) return [];
    return rawLogs.split('\n')
        .filter(line => line.trim())
        .map(line => {
            // Match K8s timestamp format: 2024-11-26T14:30:00.123456789Z
            const match = line.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d+Z)\s*(.*)/);
            if (match) {
                return { timestamp: match[1], content: match[2], source };
            }
            // Line without timestamp (shouldn't happen if we always fetch with timestamps)
            return { timestamp: '', content: line, source };
        });
};

export default function LogViewer({ namespace, pod, containers = [], siblingPods = [], podContainerMap = {}, ownerName = '', podCreationTime = '', tabContext = '' }) {
    const { currentContext } = useK8s();
    const { isDebugMode } = useDebug();
    const [logs, setLogs] = useState([]); // Array of { timestamp, content, source }
    const [loading, setLoading] = useState(false);
    const [downloading, setDownloading] = useState(false);
    const [downloadingBundle, setDownloadingBundle] = useState(false);
    const [selectedPod, setSelectedPod] = useState(pod);
    const [selectedContainer, setSelectedContainer] = useState(containers[0] || '');
    const [wrapLines, setWrapLines] = useState(true);
    const [showTimestamps, setShowTimestamps] = useState(false);
    const [showPrevious, setShowPrevious] = useState(false);
    const [showTimeModal, setShowTimeModal] = useState(false);
    const [sinceTime, setSinceTime] = useState('');
    const [viewMode, setViewMode] = useState('end'); // 'start' or 'end'
    const [autoFollow, setAutoFollow] = useState(true); // User can toggle this
    const [streamDisconnected, setStreamDisconnected] = useState(false); // Track if stream was disconnected
    const [disconnectReason, setDisconnectReason] = useState(''); // Reason for disconnection
    const [hasMoreBefore, setHasMoreBefore] = useState(false); // More logs available before current view
    const [hasMoreAfter, setHasMoreAfter] = useState(false); // More logs available after current view
    const [loadingBefore, setLoadingBefore] = useState(false); // Loading older logs (for UI)
    const [loadingAfter, setLoadingAfter] = useState(false); // Loading newer logs (for UI)
    const logsStartRef = useRef(null);
    const logsEndRef = useRef(null);
    const streamIdRef = useRef(null);
    const logsContainerRef = useRef(null);
    const isAtBottomRef = useRef(true);
    // Use refs for loading locks to prevent race conditions (React state is async)
    const loadingBeforeRef = useRef(false);
    const loadingAfterRef = useRef(false);
    const isChunkLoadingRef = useRef(false); // Skip auto-scroll during chunk loads
    // Track last fetched timestamps to avoid duplicate fetches
    const lastFetchedBeforeTs = useRef('');
    const lastFetchedAfterTs = useRef('');

    const CHUNK_SIZE = 200; // Number of lines to load per chunk

    // Extract first timestamp from current logs (for loading older logs)
    const getFirstTimestamp = () => {
        if (!logs.length) return '';
        // Find first entry with a timestamp
        const entry = logs.find(e => e.timestamp);
        return entry?.timestamp || '';
    };

    // Extract last timestamp from current logs (for loading newer logs)
    const getLastTimestamp = () => {
        if (!logs.length) return '';
        // Find last entry with a timestamp (search backwards)
        for (let i = logs.length - 1; i >= 0; i--) {
            if (logs[i].timestamp) return logs[i].timestamp;
        }
        return '';
    };

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    // Auto-follow is active when in 'end' mode with no custom time filter AND user has it enabled AND not stale
    const isFollowing = viewMode === 'end' && !sinceTime && !showPrevious && autoFollow && !isStale;

    // Stop log stream when tab becomes stale
    useEffect(() => {
        if (isStale && streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }
    }, [isStale]);

    // Track scroll position and trigger chunk loading
    const handleScroll = () => {
        if (!logsContainerRef.current) return;
        const { scrollTop, scrollHeight, clientHeight } = logsContainerRef.current;

        const distanceFromBottom = scrollHeight - scrollTop - clientHeight;

        // Consider "at bottom" if within 50px of the bottom
        isAtBottomRef.current = distanceFromBottom < 50;

        // Load older logs when near top (within 100px) - works even when following
        // Use ref for instant check to prevent race conditions
        if (scrollTop < 100 && hasMoreBefore && !loadingBeforeRef.current) {
            loadOlderLogs();
        }

        // When user scrolls away from bottom (more than 200px), reset hasMoreAfter
        // This allows loading more logs when they scroll back down
        // (useful for active pods that keep generating logs)
        if (distanceFromBottom > 200 && !hasMoreAfter && !isFollowing) {
            setHasMoreAfter(true);
            lastFetchedAfterTs.current = ''; // Reset to allow new fetch
        }

        // Load newer logs when near bottom (within 100px) and not following
        if (distanceFromBottom < 100 && hasMoreAfter && !loadingAfterRef.current && !isFollowing) {
            loadNewerLogs();
        }
    };

    useEffect(() => {
        if (namespace && selectedPod) {
            fetchLogs();
        }
        // Note: showTimestamps is NOT a dependency - we always fetch with timestamps
        // and just show/hide them in render
    }, [namespace, selectedPod, selectedContainer, showPrevious, sinceTime, viewMode]);

    // Listen for Cmd+R / Ctrl+R to refresh logs
    useEffect(() => {
        const handleKeyDown = (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'r') {
                if (namespace && selectedPod) {
                    fetchLogs();
                }
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [namespace, selectedPod, selectedContainer, showPrevious, sinceTime, viewMode]);

    const fetchLogs = async () => {
        // Stop any existing stream when fetching new logs
        if (streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }

        // Clear disconnected state when refreshing
        setStreamDisconnected(false);
        setDisconnectReason('');

        setLoading(true);
        try {
            let logData;
            if (viewMode === 'start') {
                logData = await GetPodLogsFromStart(namespace, selectedPod, selectedContainer, true, showPrevious);
                // When starting from beginning, there are likely more logs after
                setHasMoreBefore(false);
                setHasMoreAfter(true); // Assume there's more until we know otherwise
            } else {
                logData = await GetPodLogs(namespace, selectedPod, selectedContainer, true, showPrevious, sinceTime);
                // When loading from end/sinceTime, there are likely more logs before
                setHasMoreBefore(true); // Assume there's more until we know otherwise
                setHasMoreAfter(false);
            }
            setLogs(parseLogLines(logData, 'initial'));
        } catch (err) {
            setLogs([{ timestamp: '', content: `Error fetching logs: ${err}`, source: 'error' }]);
            setHasMoreBefore(false);
            setHasMoreAfter(false);
        } finally {
            setLoading(false);
        }
    };

    // Load older logs (when scrolling to top)
    const loadOlderLogs = async () => {
        // Use ref for instant lock check to prevent race conditions
        if (loadingBeforeRef.current || !hasMoreBefore) return;

        const firstTs = getFirstTimestamp();
        if (!firstTs) {
            setHasMoreBefore(false);
            return;
        }

        // Avoid duplicate fetches with the same timestamp
        if (firstTs === lastFetchedBeforeTs.current) {
            console.log('DEBUG: Skipping duplicate BEFORE fetch for', firstTs);
            return;
        }
        lastFetchedBeforeTs.current = firstTs;

        // Set locks immediately (sync) before any async operation
        loadingBeforeRef.current = true;
        isChunkLoadingRef.current = true; // Disable auto-scroll
        setLoadingBefore(true); // For UI display

        // Capture scroll position BEFORE the async call
        const container = logsContainerRef.current;
        const prevScrollHeight = container?.scrollHeight || 0;
        const prevScrollTop = container?.scrollTop || 0;

        // Stop momentum scrolling by freezing position
        if (container) {
            container.style.overflow = 'hidden';
            requestAnimationFrame(() => {
                if (container) container.style.overflow = 'auto';
            });
        }

        try {
            const result = await GetPodLogsBefore(
                namespace, selectedPod, selectedContainer,
                true, showPrevious, firstTs, CHUNK_SIZE
            );

            if (result.logs && result.logs.trim()) {
                const newEntries = parseLogLines(result.logs, 'before');
                setLogs(prev => [...newEntries, ...prev]);
                setHasMoreBefore(result.hasMore);
                // After loading older logs, enable loading newer (in case new logs appeared)
                if (!isFollowing) {
                    setHasMoreAfter(true);
                }

                // Restore scroll position after DOM update - stay in same place
                // Use double requestAnimationFrame to ensure DOM has updated
                requestAnimationFrame(() => {
                    requestAnimationFrame(() => {
                        if (container) {
                            const newScrollHeight = container.scrollHeight;
                            const addedHeight = newScrollHeight - prevScrollHeight;
                            container.scrollTop = prevScrollTop + addedHeight;
                        }
                        isChunkLoadingRef.current = false; // Re-enable auto-scroll
                    });
                });
            } else {
                setHasMoreBefore(false);
                isChunkLoadingRef.current = false;
            }
        } catch (err) {
            console.error('Failed to load older logs:', err);
            setHasMoreBefore(false);
            isChunkLoadingRef.current = false;
        } finally {
            loadingBeforeRef.current = false;
            setLoadingBefore(false);
        }
    };

    // Load newer logs (when scrolling to bottom, not following)
    const loadNewerLogs = async () => {
        // Use ref for instant lock check to prevent race conditions
        if (loadingAfterRef.current || !hasMoreAfter || isFollowing) return;

        const lastTs = getLastTimestamp();
        if (!lastTs) {
            setHasMoreAfter(false);
            return;
        }

        // Avoid duplicate fetches with the same timestamp
        if (lastTs === lastFetchedAfterTs.current) {
            console.log('DEBUG: Skipping duplicate AFTER fetch for', lastTs);
            return;
        }
        lastFetchedAfterTs.current = lastTs;

        // Set locks immediately (sync) before any async operation
        loadingAfterRef.current = true;
        isChunkLoadingRef.current = true; // Disable auto-scroll
        setLoadingAfter(true); // For UI display

        // Stop momentum scrolling by freezing position
        const container = logsContainerRef.current;
        if (container) {
            container.style.overflow = 'hidden';
            requestAnimationFrame(() => {
                if (container) container.style.overflow = 'auto';
            });
        }

        try {
            const result = await GetPodLogsAfter(
                namespace, selectedPod, selectedContainer,
                true, showPrevious, lastTs, CHUNK_SIZE
            );

            if (result.logs && result.logs.trim()) {
                const newEntries = parseLogLines(result.logs, 'after');
                setLogs(prev => [...prev, ...newEntries]);
                setHasMoreAfter(result.hasMore);
            } else {
                setHasMoreAfter(false);
            }
        } catch (err) {
            console.error('Failed to load newer logs:', err);
            setHasMoreAfter(false);
        } finally {
            loadingAfterRef.current = false;
            isChunkLoadingRef.current = false;
            setLoadingAfter(false);
        }
    };

    // Log streaming for auto-follow
    useEffect(() => {
        // Stop existing stream first
        if (streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }

        // Start streaming if following is active
        if (isFollowing && namespace && selectedPod && !loading) {
            const startStream = async () => {
                try {
                    // Always stream with timestamps for chunk loading consistency (display strips them)
                    const streamId = await StartLogStream(namespace, selectedPod, selectedContainer, true);
                    streamIdRef.current = streamId;
                } catch (err) {
                    console.error('Failed to start log stream:', err);
                }
            };
            startStream();
        }

        // Cleanup on unmount or when dependencies change
        return () => {
            if (streamIdRef.current) {
                StopLogStream(streamIdRef.current);
                streamIdRef.current = null;
            }
        };
    }, [isFollowing, namespace, selectedPod, selectedContainer, showTimestamps, loading]);

    // Listen for log stream events
    useEffect(() => {
        const handleLogEvent = (event) => {
            // Only process events for our current stream
            if (!streamIdRef.current || event.streamId !== streamIdRef.current) return;

            if (event.done) {
                // Stream ended (pod terminated, etc.)
                streamIdRef.current = null;
                setStreamDisconnected(true);
                setDisconnectReason('Pod terminated or container restarted');
                return;
            }

            if (event.error) {
                console.error('Log stream error:', event.error);
                streamIdRef.current = null;
                setStreamDisconnected(true);
                setDisconnectReason(event.error);
                return;
            }

            if (event.line) {
                // Parse the streamed line and append as structured entry
                const newEntries = parseLogLines(event.line, 'stream');
                if (newEntries.length > 0) {
                    setLogs(prev => {
                        // Filter out stream lines older than what we already have
                        // This prevents duplicates when stream starts with backlog
                        const lastTs = prev.length > 0 ?
                            (prev.slice().reverse().find(e => e.timestamp)?.timestamp || '') : '';

                        const filteredEntries = lastTs ?
                            newEntries.filter(e => !e.timestamp || e.timestamp > lastTs) :
                            newEntries;

                        return filteredEntries.length > 0 ? [...prev, ...filteredEntries] : prev;
                    });
                }
            }
        };

        EventsOn('log-stream', handleLogEvent);

        return () => {
            EventsOff('log-stream');
        };
    }, []);

    // Auto-scroll to bottom only when user was already at bottom, or on initial load/mode change
    // Skip auto-scroll during chunk loading (we handle scroll position manually)
    const prevLogsLengthRef = useRef(0);
    useEffect(() => {
        // Skip auto-scroll during chunk loading
        if (isChunkLoadingRef.current) {
            prevLogsLengthRef.current = logs.length;
            return;
        }

        const isInitialLoad = prevLogsLengthRef.current === 0 && logs.length > 0;
        const isNewFetch = logs.length > 0 && prevLogsLengthRef.current === 0;

        // Only auto-scroll on initial load or fresh fetch, not on chunk loads
        if (viewMode === 'start' && logsContainerRef.current && (isInitialLoad || isNewFetch)) {
            logsContainerRef.current.scrollTop = 0;
            isAtBottomRef.current = false;
        } else if (logsContainerRef.current && (isAtBottomRef.current || isInitialLoad || isNewFetch)) {
            logsContainerRef.current.scrollTop = logsContainerRef.current.scrollHeight;
            isAtBottomRef.current = true;
        }

        prevLogsLengthRef.current = logs.length;
    }, [logs, viewMode]);

    // Convert structured logs to HTML for display
    const renderLogs = () => {
        if (!logs || logs.length === 0) return null;

        return logs.map((entry, index) => {
            // Convert ANSI codes in content
            const htmlContent = { __html: converter.toHtml(normalizeAnsiCodes(entry.content)) };

            return (
                <div key={index} className="flex">
                    {showTimestamps && entry.timestamp && (
                        <span className="text-gray-500 select-none mr-2 shrink-0">
                            {entry.timestamp}
                        </span>
                    )}
                    <span dangerouslySetInnerHTML={htmlContent} />
                </div>
            );
        });
    };

    const jumpToStart = () => {
        // Reset chunk loading state to prevent races
        loadingBeforeRef.current = false;
        loadingAfterRef.current = false;
        isChunkLoadingRef.current = false;
        lastFetchedBeforeTs.current = '';
        lastFetchedAfterTs.current = '';
        setLoadingBefore(false);
        setLoadingAfter(false);

        // Clear logs immediately and reset pagination state
        setLogs([]);
        setHasMoreBefore(false);
        setHasMoreAfter(true);

        setViewMode('start');
        setSinceTime(''); // Clear time filter when jumping to start
    };

    // Force scroll to bottom
    const scrollToBottom = () => {
        isAtBottomRef.current = true;
        if (logsContainerRef.current) {
            logsContainerRef.current.scrollTop = logsContainerRef.current.scrollHeight;
        }
    };

    const jumpToEnd = () => {
        // If already in end mode, toggle auto-follow
        if (viewMode === 'end' && !sinceTime && !showPrevious) {
            const newAutoFollow = !autoFollow;
            setAutoFollow(newAutoFollow);
            if (newAutoFollow) {
                // Re-enabling auto-follow, scroll to bottom and stop manual loading
                scrollToBottom();
                setHasMoreAfter(false);
            } else {
                // Disabling auto-follow - enable manual loading of newer logs
                setHasMoreAfter(true);
            }
            return;
        }

        // Switching to end mode - reset chunk loading state to prevent races
        loadingBeforeRef.current = false;
        loadingAfterRef.current = false;
        isChunkLoadingRef.current = false;
        lastFetchedBeforeTs.current = '';
        lastFetchedAfterTs.current = '';
        setLoadingBefore(false);
        setLoadingAfter(false);

        // Clear logs and reset pagination state
        setLogs([]);
        setHasMoreBefore(true);
        setHasMoreAfter(false);

        setViewMode('end');
        setSinceTime(''); // Clear time filter to show latest logs
        setAutoFollow(true); // Enable auto-follow
        scrollToBottom();
    };

    const downloadLogs = async () => {
        setDownloading(true);
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `${selectedPod}-${timestamp}.log`;
        try {
            const allLogs = await GetAllPodLogs(namespace, selectedPod, selectedContainer, showTimestamps, showPrevious);
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
                        entries.push({
                            podName,
                            containerName,
                            logs: logs || ''
                        });
                    } catch (err) {
                        console.error(`Failed to get logs for ${podName}/${containerName}:`, err);
                        entries.push({
                            podName,
                            containerName,
                            logs: `Error fetching logs: ${err}`
                        });
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

    // Convert structured logs to visible format (what user sees - respects showTimestamps setting)
    const logsToVisibleString = () => {
        return logs.map(entry => {
            if (showTimestamps && entry.timestamp) {
                return `${entry.timestamp} ${entry.content}`;
            }
            return entry.content;
        }).join('\n');
    };

    // Convert structured logs to debug format (includes source markers)
    const logsToDebugString = () => {
        return logs.map(entry => {
            const sourceMarker = `[${entry.source.toUpperCase()}]`;
            if (entry.timestamp) {
                return `${entry.timestamp} ${sourceMarker} ${entry.content}`;
            }
            return `${sourceMarker} ${entry.content}`;
        }).join('\n');
    };

    // Download currently visible logs (what user sees)
    const downloadVisibleLogs = async () => {
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `${selectedPod}-${timestamp}.log`;
        try {
            await SavePodLogs(logsToVisibleString(), filename);
        } catch (err) {
            console.error('Failed to save visible logs:', err);
        }
    };

    // DEBUG: Download currently loaded logs with debug markers
    const downloadDebugLogs = async () => {
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `DEBUG-${selectedPod}-${timestamp}.log`;
        try {
            await SavePodLogs(logsToDebugString(), filename);
        } catch (err) {
            console.error('Failed to save debug logs:', err);
        }
    };

    // Time Picker Modal Component
    const TimePickerModal = () => {
        const [inputTime, setInputTime] = useState('');
        const [error, setError] = useState('');

        // Pre-fill when modal opens: current filter, first visible timestamp from logs, or pod creation time
        // Only run when modal opens, not on every log update
        const prevShowModal = useRef(false);
        useEffect(() => {
            // Only initialize when modal transitions from closed to open
            if (showTimeModal && !prevShowModal.current) {
                setError('');
                if (sinceTime) {
                    setInputTime(sinceTime.replace('T', ' ').replace('Z', ''));
                } else {
                    // Get first timestamp from structured logs
                    const firstTs = getFirstTimestamp();
                    if (firstTs) {
                        // Format timestamp for input: YYYY-MM-DD HH:MM:SS
                        setInputTime(firstTs.replace('T', ' ').replace(/\.\d+Z$/, ''));
                    } else if (podCreationTime) {
                        // Fallback to pod creation time (format: 2024-11-26T14:30:00Z)
                        setInputTime(podCreationTime.replace('T', ' ').replace('Z', '').slice(0, 19));
                    } else {
                        setInputTime('');
                    }
                }
            }
            prevShowModal.current = showTimeModal;
        }, [showTimeModal]);

        const handleApply = () => {
            if (!inputTime) {
                setError('Please enter a date/time');
                return;
            }
            if (!isValidDateTime(inputTime)) {
                setError('Invalid format. Use: YYYY-MM-DD HH:MM:SS');
                return;
            }
            setSinceTime(toRFC3339(inputTime));
            setViewMode('end'); // Switch to end mode when filtering by time
            setShowTimeModal(false);
        };

        if (!showTimeModal) return null;

        return createPortal(
            <div className="fixed inset-0 z-[100] flex items-center justify-center">
                <div className="absolute inset-0 bg-black/50" onClick={() => setShowTimeModal(false)} />
                <div className="relative bg-[#2d2d2d] border border-[#3d3d3d] rounded-lg shadow-xl max-w-sm w-full mx-4 p-4">
                    <h3 className="text-sm font-medium text-white mb-2">Jump to Time</h3>
                    <p className="text-xs text-gray-400 mb-3">Show logs starting from this time (server time).</p>
                    <input
                        type="text"
                        value={inputTime}
                        onChange={(e) => {
                            setInputTime(e.target.value);
                            setError('');
                        }}
                        placeholder="YYYY-MM-DD HH:MM:SS"
                        className="w-full px-3 py-2 text-sm bg-[#1e1e1e] border border-[#3d3d3d] rounded text-white mb-1 font-mono"
                        autoFocus
                    />
                    {error && <p className="text-xs text-red-400 mb-2">{error}</p>}
                    <p className="text-xs text-gray-500 mb-3">Example: 2024-11-26 14:30:00</p>
                    <div className="flex justify-end gap-2">
                        <button
                            onClick={() => setShowTimeModal(false)}
                            className="px-3 py-1.5 text-xs text-gray-400 hover:text-white transition-colors"
                        >
                            Cancel
                        </button>
                        <button
                            onClick={handleApply}
                            className="px-3 py-1.5 text-xs bg-primary text-white rounded hover:bg-primary/80 transition-colors"
                        >
                            Apply
                        </button>
                    </div>
                </div>
            </div>,
            document.body
        );
    };

    return (
        <div className="flex flex-col h-full bg-[#1e1e1e]">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-red-900/30 border-b border-red-500/50 text-red-400 shrink-0">
                    <ExclamationTriangleIcon className="h-5 w-5" />
                    <span className="text-sm">
                        Read-only: These logs are from context <span className="font-medium">{tabContext}</span>. Switch back to view live logs.
                    </span>
                </div>
            )}

            {/* Stream Disconnected Banner */}
            {streamDisconnected && !isStale && (
                <div className="flex items-center justify-between px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <div className="flex items-center gap-2">
                        <ExclamationTriangleIcon className="h-5 w-5" />
                        <span className="text-sm">
                            {disconnectReason || 'Stream disconnected'}
                        </span>
                    </div>
                    <button
                        onClick={fetchLogs}
                        disabled={loading}
                        className="flex items-center gap-1.5 px-3 py-1 text-xs font-medium bg-amber-600 text-white rounded hover:bg-amber-500 transition-colors disabled:opacity-50"
                    >
                        <ArrowPathIcon className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
                        Refresh
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
                                options={siblingPods.length > 0 ? siblingPods : [pod]}
                                value={selectedPod}
                                onChange={setSelectedPod}
                                placeholder="Select Pod..."
                                className="text-xs"
                            />
                        </div>
                    </div>
                    {containers.length > 0 && (
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-500">Container:</span>
                            <div className="w-48">
                                <SearchSelect
                                    options={containers}
                                    value={selectedContainer}
                                    onChange={setSelectedContainer}
                                    placeholder="Select Container..."
                                    className="text-xs"
                                />
                            </div>
                        </div>
                    )}
                </div>
                <div className="flex items-center gap-1">
                    {/* Wrap Lines Toggle */}
                    <button
                        onClick={() => setWrapLines(!wrapLines)}
                        className={`p-1.5 rounded transition-colors ${wrapLines ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        title={wrapLines ? 'Disable line wrap' : 'Enable line wrap'}
                    >
                        <Bars3BottomLeftIcon className="w-4 h-4" />
                    </button>

                    {/* Timestamps Toggle */}
                    <button
                        onClick={() => setShowTimestamps(!showTimestamps)}
                        className={`p-1.5 rounded transition-colors ${showTimestamps ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        title={showTimestamps ? 'Hide timestamps' : 'Show timestamps'}
                    >
                        <ClockIcon className="w-4 h-4" />
                    </button>

                    {/* Previous Container Logs Toggle */}
                    <button
                        onClick={() => setShowPrevious(!showPrevious)}
                        className={`p-1.5 rounded transition-colors ${showPrevious ? 'bg-amber-500/20 text-amber-400' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        title={showPrevious ? 'Showing previous container logs' : 'Show previous container logs'}
                    >
                        <BackwardIcon className="w-4 h-4" />
                    </button>

                    <div className="w-px h-4 bg-border mx-1" />

                    {/* Jump to Start */}
                    <button
                        onClick={jumpToStart}
                        disabled={loading}
                        className={`p-1.5 rounded transition-colors disabled:opacity-50 ${viewMode === 'start' ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        title="Jump to start"
                    >
                        <ChevronDoubleUpIcon className="w-4 h-4" />
                    </button>

                    {/* Jump to Time */}
                    <button
                        onClick={() => setShowTimeModal(true)}
                        className={`p-1.5 rounded transition-colors ${sinceTime ? 'bg-green-500/20 text-green-400' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        title={sinceTime ? `Filtering from: ${sinceTime}` : 'Jump to time'}
                    >
                        <CalendarIcon className="w-4 h-4" />
                    </button>

                    {/* Jump to End / Follow */}
                    <button
                        onClick={jumpToEnd}
                        disabled={loading}
                        className={`p-1.5 rounded transition-colors disabled:opacity-50 ${isFollowing ? 'bg-green-500/20 text-green-400' : viewMode === 'end' && !sinceTime ? 'bg-primary/20 text-primary' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                        title={isFollowing ? "Following logs (click to pause)" : viewMode === 'end' && !sinceTime && !showPrevious ? "Click to resume following" : "Jump to end & follow"}
                    >
                        <ChevronDoubleDownIcon className={`w-4 h-4 ${isFollowing ? 'animate-pulse' : ''}`} />
                    </button>

                    <div className="w-px h-4 bg-border mx-1" />

                    {/* Download */}
                    <button
                        onClick={downloadLogs}
                        disabled={logs.length === 0 || loading || downloading}
                        className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        title="Download container logs"
                    >
                        {downloading ? <Spinner /> : <ArrowDownTrayIcon className="w-4 h-4" />}
                    </button>

                    {/* Download All */}
                    {siblingPods.length > 1 && (
                        <button
                            onClick={downloadBundle}
                            disabled={loading || downloadingBundle}
                            className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            title="Download all pod logs"
                        >
                            {downloadingBundle ? <Spinner /> : <ArchiveBoxArrowDownIcon className="w-4 h-4" />}
                        </button>
                    )}

                    {/* Download visible logs (what user currently sees) */}
                    <button
                        onClick={downloadVisibleLogs}
                        disabled={logs.length === 0}
                        className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                        title="Download currently visible logs"
                    >
                        <EyeIcon className="w-4 h-4" />
                    </button>

                    {/* DEBUG: Download logs with debug markers (only visible in debug mode) */}
                    {isDebugMode && (
                        <button
                            onClick={downloadDebugLogs}
                            disabled={logs.length === 0}
                            className="p-1.5 rounded text-amber-400 hover:text-amber-300 hover:bg-amber-500/10 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                            title="DEBUG: Download logs with source markers"
                        >
                            <BugAntIcon className="w-4 h-4" />
                        </button>
                    )}
                </div>
            </div>

            {/* Logs Content */}
            <div
                ref={logsContainerRef}
                onScroll={handleScroll}
                className="flex-1 overflow-auto p-4 text-gray-300 font-mono text-xs"
            >
                {loading ? (
                    <div className="flex items-center justify-center h-full">
                        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                    </div>
                ) : (
                    <div className={wrapLines ? "whitespace-pre-wrap break-all" : "whitespace-pre"}>
                        {/* Loading older logs indicator */}
                        {loadingBefore && (
                            <div className="flex items-center justify-center py-2 text-gray-500">
                                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary mr-2"></div>
                                Loading older logs...
                            </div>
                        )}
                        {/* At the top indicator */}
                        {!hasMoreBefore && !loadingBefore && logs.length > 0 && (
                            <div className="flex items-center justify-center py-1 text-gray-600 text-xs">
                                — You are at the top —
                            </div>
                        )}
                        <div ref={logsStartRef} />
                        {logs.length > 0 ? (
                            <div>{renderLogs()}</div>
                        ) : showPrevious ? (
                            <div className="text-amber-400">No previous container logs available.</div>
                        ) : (
                            <div className="text-gray-500">No logs available.</div>
                        )}
                        <div ref={logsEndRef} />
                        {/* Loading newer logs indicator */}
                        {loadingAfter && (
                            <div className="flex items-center justify-center py-2 text-gray-500">
                                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary mr-2"></div>
                                Loading newer logs...
                            </div>
                        )}
                    </div>
                )}
            </div>

            {/* Time Picker Modal */}
            <TimePickerModal />
        </div>
    );
}
