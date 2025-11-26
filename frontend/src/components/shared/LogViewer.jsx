import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { GetPodLogs, GetAllPodLogs, GetPodLogsFromStart, SavePodLogs, SaveLogsBundle, StartLogStream, StopLogStream } from '../../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime';
import Convert from 'ansi-to-html';
import {
    ArrowDownTrayIcon,
    ArchiveBoxArrowDownIcon,
    ClockIcon,
    Bars3BottomLeftIcon,
    BackwardIcon,
    ChevronDoubleUpIcon,
    ChevronDoubleDownIcon,
    CalendarIcon
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

// Extract first timestamp from logs (Kubernetes format: 2024-11-26T14:30:00.123456789Z)
const extractFirstTimestamp = (logText) => {
    if (!logText) return '';
    const match = logText.match(/^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})/m);
    if (match) {
        // Return in user-friendly format: YYYY-MM-DD HH:MM:SS
        return match[1].replace('T', ' ');
    }
    return '';
};

export default function LogViewer({ namespace, pod, containers = [], siblingPods = [], podContainerMap = {}, ownerName = '', podCreationTime = '' }) {
    const [logs, setLogs] = useState('');
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
    const logsStartRef = useRef(null);
    const logsEndRef = useRef(null);
    const streamIdRef = useRef(null);
    const logsContainerRef = useRef(null);
    const isAtBottomRef = useRef(true);

    // Auto-follow is active when in 'end' mode with no custom time filter AND user has it enabled
    const isFollowing = viewMode === 'end' && !sinceTime && !showPrevious && autoFollow;

    // Track scroll position to determine if user is at bottom
    const handleScroll = () => {
        if (!logsContainerRef.current) return;
        const { scrollTop, scrollHeight, clientHeight } = logsContainerRef.current;
        // Consider "at bottom" if within 50px of the bottom
        isAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 50;
    };

    useEffect(() => {
        if (namespace && selectedPod) {
            fetchLogs();
        }
    }, [namespace, selectedPod, selectedContainer, showTimestamps, showPrevious, sinceTime, viewMode]);

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
    }, [namespace, selectedPod, selectedContainer, showTimestamps, showPrevious, sinceTime, viewMode]);

    const fetchLogs = async () => {
        // Stop any existing stream when fetching new logs
        if (streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }

        setLoading(true);
        try {
            let logData;
            if (viewMode === 'start') {
                logData = await GetPodLogsFromStart(namespace, selectedPod, selectedContainer, showTimestamps, showPrevious);
            } else {
                logData = await GetPodLogs(namespace, selectedPod, selectedContainer, showTimestamps, showPrevious, sinceTime);
            }
            setLogs(logData);
        } catch (err) {
            setLogs(`Error fetching logs: ${err}`);
        } finally {
            setLoading(false);
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
                    const streamId = await StartLogStream(namespace, selectedPod, selectedContainer, showTimestamps);
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
                return;
            }

            if (event.error) {
                console.error('Log stream error:', event.error);
                return;
            }

            if (event.line) {
                setLogs(prev => prev ? prev + '\n' + event.line : event.line);
            }
        };

        EventsOn('log-stream', handleLogEvent);

        return () => {
            EventsOff('log-stream');
        };
    }, []);

    // Auto-scroll to bottom only when user was already at bottom, or on initial load/mode change
    const prevLogsRef = useRef('');
    useEffect(() => {
        const isInitialLoad = !prevLogsRef.current && logs;
        const isNewFetch = prevLogsRef.current !== logs && !prevLogsRef.current.length;

        if (viewMode === 'start' && logsStartRef.current) {
            logsStartRef.current.scrollIntoView({ behavior: "smooth" });
            isAtBottomRef.current = false;
        } else if (logsEndRef.current && (isAtBottomRef.current || isInitialLoad || isNewFetch)) {
            logsEndRef.current.scrollIntoView({ behavior: "smooth" });
            isAtBottomRef.current = true;
        }

        prevLogsRef.current = logs;
    }, [logs, viewMode]);

    const getHtmlLogs = () => {
        if (!logs) return { __html: "" };
        return { __html: converter.toHtml(normalizeAnsiCodes(logs)) };
    };

    const jumpToStart = () => {
        setViewMode('start');
        setSinceTime(''); // Clear time filter when jumping to start
    };

    const jumpToEnd = () => {
        // If already in end mode, toggle auto-follow
        if (viewMode === 'end' && !sinceTime && !showPrevious) {
            setAutoFollow(prev => !prev);
            if (!autoFollow) {
                // Re-enabling auto-follow, scroll to bottom
                isAtBottomRef.current = true;
                if (logsEndRef.current) {
                    logsEndRef.current.scrollIntoView({ behavior: "smooth" });
                }
            }
            return;
        }

        // Switching to end mode
        setViewMode('end');
        setSinceTime(''); // Clear time filter to show latest logs
        setAutoFollow(true); // Enable auto-follow
        isAtBottomRef.current = true;
        // Scroll to bottom immediately
        setTimeout(() => {
            if (logsEndRef.current) {
                logsEndRef.current.scrollIntoView({ behavior: "smooth" });
            }
        }, 0);
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
                    const extracted = extractFirstTimestamp(logs);
                    if (extracted) {
                        setInputTime(extracted);
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
                        disabled={!logs || loading || downloading}
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
                        <div ref={logsStartRef} />
                        {logs ? (
                            <div dangerouslySetInnerHTML={getHtmlLogs()} />
                        ) : showPrevious ? (
                            <div className="text-amber-400">No previous container logs available.</div>
                        ) : (
                            <div className="text-gray-500">No logs available.</div>
                        )}
                        <div ref={logsEndRef} />
                    </div>
                )}
            </div>

            {/* Time Picker Modal */}
            <TimePickerModal />
        </div>
    );
}
