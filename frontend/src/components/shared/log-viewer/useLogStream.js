import { useState, useEffect, useRef, useCallback } from 'react';
import {
    GetPodLogs, GetAllPodLogs, GetPodLogsFromStart, GetPodLogsBefore, GetPodLogsAfter,
    StartLogStream, StopLogStream,
    GetAllContainersLogs, GetAllContainersLogsAll, GetAllContainersLogsFromStart,
    GetAllContainersLogsBefore, GetAllContainersLogsAfter, StartAllContainersLogStream,
    GetAllPodsLogs, GetAllPodsLogsAll, GetAllPodsLogsFromStart,
    GetAllPodsLogsBefore, GetAllPodsLogsAfter, StartAllPodsLogStream
} from '../../../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../../../wailsjs/runtime/runtime';
import { parseLogLines } from './logUtils';

// Special constants for "All" modes
export const ALL_CONTAINERS = '__ALL__';
export const ALL_PODS = '__ALL_PODS__';

const CHUNK_SIZE = 200; // Number of lines to load per chunk

/**
 * Hook for managing log fetching, streaming, and chunk loading.
 * Handles initial load, pagination (before/after), and real-time streaming.
 * Supports "All Containers" mode when container === ALL_CONTAINERS.
 * Supports "All Pods" mode when pod === ALL_PODS.
 */
export function useLogStream({
    namespace,
    pod,
    container,
    containers, // Array of all container names (needed for "All Containers" mode)
    siblingPods, // Array of sibling pod names (needed for "All Pods" mode)
    podContainerMap, // Map of podName -> containerNames (needed for "All Pods" mode)
    showPrevious,
    sinceTime,
    viewMode,
    initialPosition,
    isStale,
    currentContext
}) {
    const isAllContainers = container === ALL_CONTAINERS;
    const isAllPods = pod === ALL_PODS;

    // Build pod-container pairs for "All Pods" mode
    const buildPodContainerPairs = useCallback(() => {
        if (!isAllPods || !siblingPods?.length) return [];
        return siblingPods.map(podName => ({
            podName,
            containerNames: isAllContainers
                ? (podContainerMap?.[podName] || containers || [])
                : (container ? [container] : [])
        }));
    }, [isAllPods, isAllContainers, siblingPods, podContainerMap, containers, container]);
    const [logs, setLogs] = useState([]); // Array of { timestamp, content, source }
    // Start with loading=true to prevent streaming race condition on initial mount
    const [loading, setLoading] = useState(true);
    const [loadingAll, setLoadingAll] = useState(false);
    const [loadingBefore, setLoadingBefore] = useState(false);
    const [loadingAfter, setLoadingAfter] = useState(false);
    const [hasMoreBefore, setHasMoreBefore] = useState(false);
    const [hasMoreAfter, setHasMoreAfter] = useState(false);
    const [isAllLoaded, setIsAllLoaded] = useState(() => initialPosition === 'all');
    const [streamDisconnected, setStreamDisconnected] = useState(false);
    const [disconnectReason, setDisconnectReason] = useState('');
    const [firstItemIndex, setFirstItemIndex] = useState(10000); // For virtuoso prepending

    const streamIdRef = useRef(null);
    const loadingBeforeRef = useRef(false);
    const loadingAfterRef = useRef(false);
    const lastFetchedBeforeTs = useRef('');
    const lastFetchedAfterTs = useRef('');
    const initialLoadDone = useRef(false);
    const prevPodRef = useRef(pod);
    const prevContainerRef = useRef(container);
    const prevShowPreviousRef = useRef(showPrevious);
    // Ref to track fetch state synchronously (avoids React batching race conditions)
    const isFetchingRef = useRef(true);

    // Extract first timestamp from current logs
    const getFirstTimestamp = useCallback(() => {
        if (!logs.length) return '';
        const entry = logs.find(e => e.timestamp);
        return entry?.timestamp || '';
    }, [logs]);

    // Extract last timestamp from current logs
    const getLastTimestamp = useCallback(() => {
        if (!logs.length) return '';
        for (let i = logs.length - 1; i >= 0; i--) {
            if (logs[i].timestamp) return logs[i].timestamp;
        }
        return '';
    }, [logs]);

    // Reset chunk loading state
    const resetChunkState = useCallback(() => {
        loadingBeforeRef.current = false;
        loadingAfterRef.current = false;
        lastFetchedBeforeTs.current = '';
        lastFetchedAfterTs.current = '';
        setLoadingBefore(false);
        setLoadingAfter(false);
        setFirstItemIndex(10000);
    }, []);

    // Fetch logs (initial or refresh)
    const fetchLogs = useCallback(async () => {
        // Set ref immediately to prevent streaming race condition
        isFetchingRef.current = true;

        if (streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }

        setStreamDisconnected(false);
        setDisconnectReason('');
        setIsAllLoaded(false);
        setLoading(true);

        try {
            let logData;
            const podPairs = buildPodContainerPairs();

            if (viewMode === 'start') {
                if (isAllPods && podPairs.length > 0) {
                    logData = await GetAllPodsLogsFromStart(namespace, podPairs, isAllContainers, true, showPrevious);
                } else if (isAllContainers && containers?.length > 0) {
                    logData = await GetAllContainersLogsFromStart(namespace, pod, containers, true, showPrevious);
                } else {
                    logData = await GetPodLogsFromStart(namespace, pod, container, true, showPrevious);
                }
                setHasMoreBefore(false);
                setHasMoreAfter(true);
            } else {
                if (isAllPods && podPairs.length > 0) {
                    logData = await GetAllPodsLogs(namespace, podPairs, isAllContainers, true, showPrevious, sinceTime);
                } else if (isAllContainers && containers?.length > 0) {
                    logData = await GetAllContainersLogs(namespace, pod, containers, true, showPrevious, sinceTime);
                } else {
                    logData = await GetPodLogs(namespace, pod, container, true, showPrevious, sinceTime);
                }
                setHasMoreBefore(true);
                setHasMoreAfter(false);
            }
            setLogs(parseLogLines(logData, 'initial'));
        } catch (err) {
            setLogs([{ timestamp: '', content: `Error fetching logs: ${err}`, source: 'error' }]);
            setHasMoreBefore(false);
            setHasMoreAfter(false);
        } finally {
            setLoading(false);
            isFetchingRef.current = false;
        }
    }, [namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious, sinceTime, viewMode]);

    // Load all logs at once
    const loadAllLogs = useCallback(async () => {
        // Set ref immediately to prevent streaming race condition
        isFetchingRef.current = true;

        if (streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }

        resetChunkState();
        setStreamDisconnected(false);
        setDisconnectReason('');
        setLoading(true); // Also set loading for consistency with fetchLogs
        setLoadingAll(true);
        setHasMoreBefore(false);
        setHasMoreAfter(false);
        setIsAllLoaded(true);

        try {
            let allLogs;
            const podPairs = buildPodContainerPairs();

            if (isAllPods && podPairs.length > 0) {
                allLogs = await GetAllPodsLogsAll(namespace, podPairs, isAllContainers, true, showPrevious);
            } else if (isAllContainers && containers?.length > 0) {
                allLogs = await GetAllContainersLogsAll(namespace, pod, containers, true, showPrevious);
            } else {
                allLogs = await GetAllPodLogs(namespace, pod, container, true, showPrevious);
            }
            setLogs(parseLogLines(allLogs, 'initial'));
        } catch (err) {
            setLogs([{ timestamp: '', content: `Error fetching all logs: ${err}`, source: 'error' }]);
            setIsAllLoaded(false);
        } finally {
            setLoading(false);
            setLoadingAll(false);
            isFetchingRef.current = false;
        }
    }, [namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious, resetChunkState]);

    // Load older logs (when scrolling to top)
    const loadOlderLogs = useCallback(async () => {
        if (loadingBeforeRef.current || !hasMoreBefore) return;

        const firstTs = getFirstTimestamp();
        if (!firstTs) {
            setHasMoreBefore(false);
            return;
        }

        if (firstTs === lastFetchedBeforeTs.current) return;
        lastFetchedBeforeTs.current = firstTs;

        loadingBeforeRef.current = true;
        setLoadingBefore(true);

        try {
            let result;
            const podPairs = buildPodContainerPairs();

            if (isAllPods && podPairs.length > 0) {
                result = await GetAllPodsLogsBefore(
                    namespace, podPairs, isAllContainers,
                    true, showPrevious, firstTs, CHUNK_SIZE
                );
            } else if (isAllContainers && containers?.length > 0) {
                result = await GetAllContainersLogsBefore(
                    namespace, pod, containers,
                    true, showPrevious, firstTs, CHUNK_SIZE
                );
            } else {
                result = await GetPodLogsBefore(
                    namespace, pod, container,
                    true, showPrevious, firstTs, CHUNK_SIZE
                );
            }

            if (result.logs && result.logs.trim()) {
                const newEntries = parseLogLines(result.logs, 'before');
                setFirstItemIndex(prev => prev - newEntries.length);
                setLogs(prev => [...newEntries, ...prev]);
                setHasMoreBefore(result.hasMore);
            } else {
                setHasMoreBefore(false);
            }
        } catch (err) {
            console.error('Failed to load older logs:', err);
            setHasMoreBefore(false);
        } finally {
            loadingBeforeRef.current = false;
            setLoadingBefore(false);
        }
    }, [hasMoreBefore, getFirstTimestamp, namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious]);

    // Load newer logs (when scrolling to bottom, not following)
    const loadNewerLogs = useCallback(async () => {
        if (loadingAfterRef.current || !hasMoreAfter) return;

        const lastTs = getLastTimestamp();
        if (!lastTs) {
            setHasMoreAfter(false);
            return;
        }

        if (lastTs === lastFetchedAfterTs.current) return;
        lastFetchedAfterTs.current = lastTs;

        loadingAfterRef.current = true;
        setLoadingAfter(true);

        try {
            let result;
            const podPairs = buildPodContainerPairs();

            if (isAllPods && podPairs.length > 0) {
                result = await GetAllPodsLogsAfter(
                    namespace, podPairs, isAllContainers,
                    true, showPrevious, lastTs, CHUNK_SIZE
                );
            } else if (isAllContainers && containers?.length > 0) {
                result = await GetAllContainersLogsAfter(
                    namespace, pod, containers,
                    true, showPrevious, lastTs, CHUNK_SIZE
                );
            } else {
                result = await GetPodLogsAfter(
                    namespace, pod, container,
                    true, showPrevious, lastTs, CHUNK_SIZE
                );
            }

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
            setLoadingAfter(false);
        }
    }, [hasMoreAfter, getLastTimestamp, namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious]);

    // Handle pod/container/showPrevious changes
    useEffect(() => {
        // For "All Pods" mode, we only need namespace and siblingPods
        if (!namespace) return;
        if (!isAllPods && !pod) return;
        if (isAllPods && (!siblingPods || siblingPods.length === 0)) return;

        const podChanged = prevPodRef.current !== pod;
        const containerChanged = prevContainerRef.current !== container;
        const showPreviousChanged = prevShowPreviousRef.current !== showPrevious;
        prevPodRef.current = pod;
        prevContainerRef.current = container;
        prevShowPreviousRef.current = showPrevious;

        if (podChanged || containerChanged || showPreviousChanged) {
            // Set ref and loading immediately to prevent streaming race condition
            isFetchingRef.current = true;
            setLoading(true);
            resetChunkState();
            setLogs([]);
            if (isAllLoaded) {
                loadAllLogs();
            } else {
                fetchLogs();
            }
            return;
        }

        if (!initialLoadDone.current && initialPosition === 'all') {
            initialLoadDone.current = true;
            loadAllLogs();
            return;
        }

        if (isAllLoaded) return;

        initialLoadDone.current = true;
        fetchLogs();
    }, [namespace, pod, container, siblingPods, isAllPods, showPrevious, sinceTime, viewMode, isAllLoaded, initialPosition, fetchLogs, loadAllLogs, resetChunkState]);

    // Stop stream when tab becomes stale
    useEffect(() => {
        if (isStale && streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }
    }, [isStale]);

    // Start/stop log streaming
    const startStreaming = useCallback(async () => {
        if (streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }

        try {
            let streamId;
            const podPairs = buildPodContainerPairs();

            if (isAllPods && podPairs.length > 0) {
                streamId = await StartAllPodsLogStream(namespace, podPairs, isAllContainers, true);
            } else if (isAllContainers && containers?.length > 0) {
                streamId = await StartAllContainersLogStream(namespace, pod, containers, true);
            } else {
                streamId = await StartLogStream(namespace, pod, container, true);
            }
            streamIdRef.current = streamId;
        } catch (err) {
            console.error('Failed to start log stream:', err);
        }
    }, [namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs]);

    const stopStreaming = useCallback(() => {
        if (streamIdRef.current) {
            StopLogStream(streamIdRef.current);
            streamIdRef.current = null;
        }
    }, []);

    // Listen for log stream events (single lines and batches from 60fps coalescer)
    useEffect(() => {
        // Process a single log line, filtering duplicates by timestamp
        const processLine = (line) => {
            const newEntries = parseLogLines(line, 'stream');
            if (newEntries.length > 0) {
                setLogs(prev => {
                    const lastTs = prev.length > 0 ?
                        (prev.slice().reverse().find(e => e.timestamp)?.timestamp || '') : '';

                    const filteredEntries = lastTs ?
                        newEntries.filter(e => !e.timestamp || e.timestamp > lastTs) :
                        newEntries;

                    return filteredEntries.length > 0 ? [...prev, ...filteredEntries] : prev;
                });
            }
        };

        // Process multiple log lines in a single state update (from batch event)
        const processBatch = (lines) => {
            if (!lines || lines.length === 0) return;

            // Parse all lines first
            const allNewEntries = [];
            for (const line of lines) {
                const entries = parseLogLines(line, 'stream');
                allNewEntries.push(...entries);
            }

            if (allNewEntries.length > 0) {
                setLogs(prev => {
                    const lastTs = prev.length > 0 ?
                        (prev.slice().reverse().find(e => e.timestamp)?.timestamp || '') : '';

                    const filteredEntries = lastTs ?
                        allNewEntries.filter(e => !e.timestamp || e.timestamp > lastTs) :
                        allNewEntries;

                    return filteredEntries.length > 0 ? [...prev, ...filteredEntries] : prev;
                });
            }
        };

        const handleLogEvent = (event) => {
            if (!streamIdRef.current || event.streamId !== streamIdRef.current) return;

            if (event.done) {
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
                processLine(event.line);
            }
        };

        // Batch event handler (from 60fps log coalescer - reduces IPC overhead)
        const handleBatchEvent = (event) => {
            if (!streamIdRef.current || event.streamId !== streamIdRef.current) return;
            if (event.lines) {
                processBatch(event.lines);
            }
        };

        EventsOn('log-stream', handleLogEvent);
        EventsOn('log-stream-batch', handleBatchEvent);
        return () => {
            EventsOff('log-stream');
            EventsOff('log-stream-batch');
        };
    }, []);

    // Cleanup on unmount
    useEffect(() => {
        return () => {
            if (streamIdRef.current) {
                StopLogStream(streamIdRef.current);
                streamIdRef.current = null;
            }
        };
    }, []);

    return {
        logs,
        setLogs,
        loading,
        loadingAll,
        loadingBefore,
        loadingAfter,
        hasMoreBefore,
        hasMoreAfter,
        isAllLoaded,
        streamDisconnected,
        disconnectReason,
        firstItemIndex,
        setFirstItemIndex,
        setHasMoreBefore,
        setHasMoreAfter,
        setIsAllLoaded,
        fetchLogs,
        loadAllLogs,
        loadOlderLogs,
        loadNewerLogs,
        startStreaming,
        stopStreaming,
        resetChunkState,
        getFirstTimestamp,
        getLastTimestamp,
        isStreaming: () => streamIdRef.current !== null,
        // Synchronous check for fetch state (avoids React batching race)
        isFetching: () => isFetchingRef.current
    };
}
