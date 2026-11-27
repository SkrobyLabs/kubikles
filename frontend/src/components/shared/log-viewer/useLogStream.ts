import { useState, useEffect, useRef, useCallback } from 'react';
import {
    GetPodLogs, GetAllPodLogs, GetPodLogsFromStart, GetPodLogsBefore, GetPodLogsAfter,
    StartLogStream, StopLogStream,
    GetAllContainersLogs, GetAllContainersLogsAll, GetAllContainersLogsFromStart,
    GetAllContainersLogsBefore, GetAllContainersLogsAfter, StartAllContainersLogStream,
    GetAllPodsLogs, GetAllPodsLogsAll, GetAllPodsLogsFromStart,
    GetAllPodsLogsBefore, GetAllPodsLogsAfter, StartAllPodsLogStream
} from 'wailsjs/go/main/App';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { drainLogPages, parseLogLines } from './logUtils';
import { LogBuffer } from './logBuffer';
import { forgetStream, rememberStream, stopLeftoverStreams } from './streamLedger';

const stopStream = (id: string): void => {
    forgetStream(id);
    StopLogStream(id);
};

// Once per page load, before any viewer mounts: stop what a reloaded page left running. The
// flag keeps a hot module reload during development from sweeping the live page's own streams.
if (typeof window !== 'undefined' && !(window as any).__kubiklesLogStreamsSwept) {
    (window as any).__kubiklesLogStreamsSwept = true;
    stopLeftoverStreams(id => { void StopLogStream(id); });
}
import { runWithPollingBudget } from '~/utils/pollingBudget';
import { type FrameHandle, requestFrameOrTimeout } from '~/utils/frame';

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
    currentContext,
    resolveFreshLogTarget,
    isActive = true
}: { namespace: any; pod: any; container: any; containers: any; siblingPods: any; podContainerMap: any; showPrevious: any; sinceTime: any; viewMode: any; initialPosition: any; isStale: any; currentContext: any; resolveFreshLogTarget?: any; isActive?: boolean }) {
    const isAllContainers = container === ALL_CONTAINERS;
    const isAllPods = pod === ALL_PODS;

    // Build pod-container pairs for "All Pods" mode
    const buildPodContainerPairs = useCallback((overrides: any = {}) => {
        const sourceIsAllPods = overrides.isAllPods ?? isAllPods;
        const sourceIsAllContainers = overrides.isAllContainers ?? isAllContainers;
        const sourceSiblingPods = overrides.siblingPods ?? siblingPods;
        const sourcePodContainerMap = overrides.podContainerMap ?? podContainerMap;
        const sourceContainers = overrides.containers ?? containers;
        const sourceContainer = overrides.container ?? container;

        if (!sourceIsAllPods || !sourceSiblingPods?.length) return [];
        return sourceSiblingPods.map((podName: any) => ({
            podName,
            containerNames: sourceIsAllContainers
                ? (sourcePodContainerMap?.[podName] || sourceContainers || [])
                : (sourceContainer ? [sourceContainer] : [])
        }));
    }, [isAllPods, isAllContainers, siblingPods, podContainerMap, containers, container]);
    // Entries live in a bounded in-place buffer; React only sees immutable snapshots of it.
    const bufferRef = useRef<LogBuffer | null>(null);
    if (!bufferRef.current) bufferRef.current = new LogBuffer();
    const buffer = bufferRef.current;
    const [snapshot, setSnapshot] = useState(() => buffer.snapshot());
    const isActiveRef = useRef(isActive);
    isActiveRef.current = isActive;
    const pendingPublishRef = useRef(false);
    const frameRef = useRef<FrameHandle | null>(null);
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
    // Ref to track disconnected state synchronously (avoids stale closure in effects)
    const streamDisconnectedRef = useRef(false);

    const [fetchError, setFetchError] = useState<string | null>(null); // Error message when fetch fails (logs preserved)

    const streamIdRef = useRef<any>(null);
    const loadingBeforeRef = useRef(false);
    const loadingAfterRef = useRef(false);
    const pollingAfterRef = useRef(false);
    const lastFetchedBeforeTs = useRef('');
    const lastFetchedAfterTs = useRef('');
    const initialLoadDone = useRef(false);
    const prevPodRef = useRef(pod);
    const prevContainerRef = useRef(container);
    const prevShowPreviousRef = useRef(showPrevious);
    const prevSinceTimeRef = useRef(sinceTime);
    const prevViewModeRef = useRef(viewMode);
    // Ref to track fetch state synchronously (avoids React batching race conditions)
    const isFetchingRef = useRef(true);

    // Publish the buffer to React now, cancelling any frame-coalesced publish.
    const publishNow = useCallback(() => {
        frameRef.current?.cancel();
        frameRef.current = null;
        pendingPublishRef.current = false;
        const next = buffer.snapshot();
        setSnapshot(next);
        // Lines trimmed from the head can be fetched again, so the top is no longer the real top.
        if (next.trimmedTotal > 0) setHasMoreBefore(true);
    }, [buffer]);

    // Streamed appends publish at most once per animation frame, and not at all while the tab is hidden.
    const schedulePublish = useCallback(() => {
        if (!isActiveRef.current) { pendingPublishRef.current = true; return; }
        if (frameRef.current !== null) return;
        if (typeof requestAnimationFrame !== 'function') { publishNow(); return; }
        // Frames stall when the window is inactive or occluded, so a timer backs them up.
        frameRef.current = requestFrameOrTimeout(() => { frameRef.current = null; publishNow(); });
    }, [publishNow]);

    // Catch up once when the tab becomes visible again.
    useEffect(() => {
        if (isActive && pendingPublishRef.current) publishNow();
    }, [isActive, publishNow]);

    const replaceLogs = useCallback((entries: any[]) => {
        buffer.replace(entries);
        publishNow();
    }, [buffer, publishNow]);

    // Compatibility wrapper for callers that used the old React state setter.
    const setLogs = useCallback((next: any) => {
        replaceLogs(typeof next === 'function' ? next(buffer.entries) : next);
    }, [buffer, replaceLogs]);

    const getEntries = useCallback(() => buffer.entries, [buffer]);
    const getEntryById = useCallback((id: number) => buffer.entries[id - buffer.firstId], [buffer]);
    const getFirstTimestamp = useCallback(() => buffer.firstTimestamp(), [buffer]);
    const getLastTimestamp = useCallback(() => buffer.lastTimestamp, [buffer]);

    // Reset chunk loading state
    const resetChunkState = useCallback(() => {
        loadingBeforeRef.current = false;
        loadingAfterRef.current = false;
        lastFetchedBeforeTs.current = '';
        lastFetchedAfterTs.current = '';
        setLoadingBefore(false);
        setLoadingAfter(false);
    }, []);

    // Fetch logs (initial or refresh)
    const fetchLogs = useCallback(async (options: any = {}) => {
        // Set ref immediately to prevent streaming race condition
        isFetchingRef.current = true;

        if (streamIdRef.current) {
            stopStream(streamIdRef.current);
            streamIdRef.current = null;
        }

        // Clear disconnected state (user is manually refreshing)
        streamDisconnectedRef.current = false;
        setStreamDisconnected(false);
        setDisconnectReason('');
        setIsAllLoaded(false);
        setLoading(true);
        resetChunkState();

        try {
            let effectiveNamespace = namespace;
            let effectivePod = pod;
            let effectiveContainer = container;
            let effectiveContainers = containers;
            let effectiveSiblingPods = siblingPods;
            let effectivePodContainerMap = podContainerMap;
            let effectiveIsAllPods = isAllPods;
            let effectiveIsAllContainers = isAllContainers;
            let refreshedTargetChanged = false;

            if (options.refreshTarget && resolveFreshLogTarget) {
                const freshTarget = await resolveFreshLogTarget({ pod, container });
                if (freshTarget) {
                    effectiveNamespace = freshTarget.namespace ?? namespace;
                    effectivePod = freshTarget.pod ?? pod;
                    effectiveContainer = freshTarget.container ?? container;
                    effectiveContainers = freshTarget.containers ?? containers;
                    effectiveSiblingPods = freshTarget.siblingPods ?? siblingPods;
                    effectivePodContainerMap = freshTarget.podContainerMap ?? podContainerMap;
                    effectiveIsAllPods = effectivePod === ALL_PODS;
                    effectiveIsAllContainers = effectiveContainer === ALL_CONTAINERS;
                    refreshedTargetChanged = effectivePod !== pod || effectiveContainer !== container;
                }
            }

            let logData;
            const podPairs = buildPodContainerPairs({
                isAllPods: effectiveIsAllPods,
                isAllContainers: effectiveIsAllContainers,
                siblingPods: effectiveSiblingPods,
                podContainerMap: effectivePodContainerMap,
                containers: effectiveContainers,
                container: effectiveContainer,
            });

            if (viewMode === 'start') {
                if (effectiveIsAllPods && podPairs.length > 0) {
                    logData = await GetAllPodsLogsFromStart(effectiveNamespace, podPairs, effectiveIsAllContainers, true, showPrevious);
                } else if (effectiveIsAllContainers && effectiveContainers?.length > 0) {
                    logData = await GetAllContainersLogsFromStart(effectiveNamespace, effectivePod, effectiveContainers, true, showPrevious);
                } else {
                    logData = await GetPodLogsFromStart(effectiveNamespace, effectivePod, effectiveContainer, true, showPrevious);
                }
                setHasMoreBefore(false);
                setHasMoreAfter(true);
            } else {
                if (effectiveIsAllPods && podPairs.length > 0) {
                    logData = await GetAllPodsLogs(effectiveNamespace, podPairs, effectiveIsAllContainers, true, showPrevious, sinceTime);
                } else if (effectiveIsAllContainers && effectiveContainers?.length > 0) {
                    logData = await GetAllContainersLogs(effectiveNamespace, effectivePod, effectiveContainers, true, showPrevious, sinceTime);
                } else {
                    logData = await GetPodLogs(effectiveNamespace, effectivePod, effectiveContainer, true, showPrevious, sinceTime);
                }
                setHasMoreBefore(true);
                setHasMoreAfter(false);
            }
            const parsed = parseLogLines(logData, 'initial');
            if (parsed.length === 0 && buffer.length > 0 && !refreshedTargetChanged) {
                // Container gone but we have existing logs — keep them and show warning
                setFetchError('Container no longer available');
            } else {
                setFetchError(null);
                replaceLogs(parsed);
            }
        } catch (err: any) {
            // On error, preserve existing logs and show error banner
            setFetchError(`Error fetching logs: ${err}`);
            if (buffer.length === 0) replaceLogs([]);
            setHasMoreBefore(false);
            setHasMoreAfter(false);
        } finally {
            setLoading(false);
            isFetchingRef.current = false;
        }
    }, [namespace, pod, container, containers, siblingPods, podContainerMap, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious, sinceTime, viewMode, resolveFreshLogTarget, resetChunkState, buffer, replaceLogs]);

    // Load all logs at once
    const loadAllLogs = useCallback(async () => {
        // Set ref immediately to prevent streaming race condition
        isFetchingRef.current = true;

        if (streamIdRef.current) {
            stopStream(streamIdRef.current);
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
            replaceLogs(parseLogLines(allLogs, 'initial'));
            // Explicitly loaded history must survive any later live appends.
            buffer.raiseCap();
        } catch (err: any) {
            replaceLogs([{ timestamp: '', content: `Error fetching all logs: ${err}`, source: 'error' }]);
            setIsAllLoaded(false);
        } finally {
            setLoading(false);
            setLoadingAll(false);
            isFetchingRef.current = false;
        }
    }, [namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious, resetChunkState, buffer, replaceLogs]);

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
                buffer.prepend(newEntries);
                buffer.raiseCap();
                if (!result.hasMore) buffer.clearTrimmed();
                publishNow();
                setHasMoreBefore(result.hasMore);
            } else {
                // Nothing older exists any more; without this the next streamed publish
                // would see trimmedTotal > 0 and re-enable the "load older" banner.
                buffer.clearTrimmed();
                publishNow();
                setHasMoreBefore(false);
            }
        } catch (err: any) {
            console.error('Failed to load older logs:', err);
            buffer.clearTrimmed();
            publishNow();
            setHasMoreBefore(false);
        } finally {
            loadingBeforeRef.current = false;
            setLoadingBefore(false);
        }
    }, [hasMoreBefore, getFirstTimestamp, namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious, buffer, publishNow]);

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
                buffer.raiseCap();
                buffer.append(newEntries);
                publishNow();
                setHasMoreAfter(result.hasMore);
            } else {
                setHasMoreAfter(false);
            }
        } catch (err: any) {
            console.error('Failed to load newer logs:', err);
            setFetchError(`Error loading newer logs: ${err}`);
            setHasMoreAfter(false);
        } finally {
            loadingAfterRef.current = false;
            setLoadingAfter(false);
        }
    }, [hasMoreAfter, getLastTimestamp, namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious, buffer, publishNow]);

    const pollNewerLogs = useCallback(async () => {
        if (pollingAfterRef.current) return;
        const cursor = getLastTimestamp();
        if (!cursor) return;
        pollingAfterRef.current = true;
        try {
            const drained = await drainLogPages((pageCursor) => runWithPollingBudget(async () => {
                const podPairs = buildPodContainerPairs();
                if (isAllPods && podPairs.length > 0) {
                    return GetAllPodsLogsAfter(namespace, podPairs, isAllContainers, true, showPrevious, pageCursor, CHUNK_SIZE);
                } else if (isAllContainers && containers?.length > 0) {
                    return GetAllContainersLogsAfter(namespace, pod, containers, true, showPrevious, pageCursor, CHUNK_SIZE);
                }
                return GetPodLogsAfter(namespace, pod, container, true, showPrevious, pageCursor, CHUNK_SIZE);
            }), cursor);
            if (drained.entries.length > 0 && buffer.appendUnique(drained.entries).added > 0) schedulePublish();
            if (drained.truncated) setFetchError('Log polling caught up partially; remaining pages will be fetched on the next interval.');
            else setFetchError(null);
        } catch (err: any) {
            setFetchError(`Error polling logs: ${err}`);
        } finally {
            pollingAfterRef.current = false;
        }
    }, [getLastTimestamp, namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs, showPrevious, buffer, schedulePublish]);

    // Handle pod/container/showPrevious changes
    useEffect(() => {
        // For "All Pods" mode, we only need namespace and siblingPods
        if (!namespace) return;
        if (!isAllPods && !pod) return;
        if (isAllPods && (!siblingPods || siblingPods.length === 0)) return;

        const podChanged = prevPodRef.current !== pod;
        const containerChanged = prevContainerRef.current !== container;
        const showPreviousChanged = prevShowPreviousRef.current !== showPrevious;
        const sinceTimeChanged = prevSinceTimeRef.current !== sinceTime;
        const viewModeChanged = prevViewModeRef.current !== viewMode;
        prevPodRef.current = pod;
        prevContainerRef.current = container;
        prevShowPreviousRef.current = showPrevious;
        prevSinceTimeRef.current = sinceTime;
        prevViewModeRef.current = viewMode;

        if (podChanged || containerChanged || showPreviousChanged || sinceTimeChanged || viewModeChanged) {
            // Set ref and loading immediately to prevent streaming race condition
            isFetchingRef.current = true;
            // Clear disconnected state since user is changing selection
            streamDisconnectedRef.current = false;
            setStreamDisconnected(false);
            setDisconnectReason('');
            setLoading(true);
            resetChunkState();
            replaceLogs([]);
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

        // Don't auto-fetch if stream was disconnected (preserve existing logs)
        if (streamDisconnectedRef.current) return;

        // Only fetch on initial load, not on every effect run
        if (initialLoadDone.current) return;

        initialLoadDone.current = true;
        fetchLogs();
    }, [namespace, pod, container, siblingPods, isAllPods, showPrevious, sinceTime, viewMode, isAllLoaded, initialPosition, fetchLogs, loadAllLogs, resetChunkState, replaceLogs]);

    // Sequence of stream starts; see startStreaming.
    const startSeqRef = useRef(0);
    // Stop stream when tab becomes stale
    useEffect(() => {
        if (isStale) startSeqRef.current++;
        if (isStale && streamIdRef.current) {
            stopStream(streamIdRef.current);
            streamIdRef.current = null;
        }
    }, [isStale]);

    // Start/stop log streaming
    // Starts are asynchronous and can overlap (follow toggled twice, refresh during a start).
    // Each start takes a sequence number; a start that finishes after a newer one began, or after
    // a stop, stops the stream it just opened instead of leaving it running unowned.
    const startStreaming = useCallback(async () => {
        const seq = ++startSeqRef.current;
        if (streamIdRef.current) {
            stopStream(streamIdRef.current);
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
            if (seq !== startSeqRef.current) {
                if (streamId) stopStream(streamId);
                return;
            }
            streamIdRef.current = streamId;
            if (streamId) rememberStream(streamId);
        } catch (err: any) {
            console.error('Failed to start log stream:', err);
        }
    }, [namespace, pod, container, containers, isAllContainers, isAllPods, buildPodContainerPairs]);

    const stopStreaming = useCallback(() => {
        startSeqRef.current++;
        if (streamIdRef.current) {
            stopStream(streamIdRef.current);
            streamIdRef.current = null;
        }
    }, []);

    // Listen for log stream events (single lines and batches from 60fps coalescer)
    useEffect(() => {
        // Append parsed entries to the bounded buffer (dropping replays by timestamp) and
        // let the frame-coalesced publisher tell React. Cost is O(batch), not O(buffer).
        const processEntries = (entries: any[]) => {
            if (entries.length > 0 && buffer.appendLive(entries).added > 0) schedulePublish();
        };

        const processLine = (line: any) => processEntries(parseLogLines(line, 'stream'));

        const processBatch = (lines: any) => {
            if (!lines || lines.length === 0) return;
            const all: any[] = [];
            for (const line of lines) {
                const entries = parseLogLines(line, 'stream');
                for (let i = 0; i < entries.length; i++) all.push(entries[i]);
            }
            processEntries(all);
        };

        const handleLogEvent = (event: any) => {
            if (!streamIdRef.current || event.streamId !== streamIdRef.current) return;

            if (event.done) {
                forgetStream(streamIdRef.current);
                streamIdRef.current = null;
                streamDisconnectedRef.current = true;
                setStreamDisconnected(true);
                setDisconnectReason('Pod terminated or container restarted');
                return;
            }

            if (event.error) {
                console.error('Log stream error:', event.error);
                forgetStream(streamIdRef.current);
                streamIdRef.current = null;
                streamDisconnectedRef.current = true;
                setStreamDisconnected(true);
                setDisconnectReason(event.error);
                return;
            }

            if (event.line) {
                processLine(event.line);
            }
        };

        // Batch event handler (from 60fps log coalescer - reduces IPC overhead)
        const handleBatchEvent = (event: any) => {
            if (!streamIdRef.current || event.streamId !== streamIdRef.current) return;
            if (event.lines) {
                processBatch(event.lines);
            }
        };

        const cancelLogStream = EventsOn('log-stream', handleLogEvent);
        const cancelLogBatch = EventsOn('log-stream-batch', handleBatchEvent);
        return () => {
            cancelLogStream();
            cancelLogBatch();
        };
    }, [buffer, schedulePublish]);

    // Cleanup on unmount
    useEffect(() => {
        return () => {
            frameRef.current?.cancel();
            frameRef.current = null;
            startSeqRef.current++;
            if (streamIdRef.current) {
                stopStream(streamIdRef.current);
                streamIdRef.current = null;
            }
        };
    }, []);

    return {
        // Backing array. It is appended to in place: use `logCount` for the published length.
        logs: snapshot.entries,
        logCount: snapshot.count,
        firstItemIndex: snapshot.firstId,
        trimmedCount: snapshot.trimmedTotal,
        // Live view of the buffer for actions that must see unpublished lines (download, copy).
        getEntries,
        getEntryById,
        setLogs,
        loading,
        loadingAll,
        loadingBefore,
        loadingAfter,
        hasMoreBefore,
        hasMoreAfter,
        isAllLoaded,
        fetchError,
        setFetchError,
        clearFetchError: () => setFetchError(null),
        streamDisconnected,
        disconnectReason,
        setHasMoreBefore,
        setHasMoreAfter,
        setIsAllLoaded,
        fetchLogs,
        loadAllLogs,
        loadOlderLogs,
        loadNewerLogs,
        pollNewerLogs,
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
