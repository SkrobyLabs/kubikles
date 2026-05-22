import React, { useState, useEffect, useRef, useCallback } from 'react';
import { Virtuoso } from 'react-virtuoso';
import { GetAllPodLogs, SavePodLogs, SaveLogsBundle } from 'wailsjs/go/main/App';
import { useK8s, useConfig } from '~/context';
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
import { logsToVisibleString, logsToDebugString, stripAnsiCodes } from './logUtils';
import { GetAllContainersLogsAll, GetAllPodsLogsAll } from 'wailsjs/go/main/App';

export default function LogViewer({
    namespace: initialNamespace,
    pod: initialPod,
    containers: initialContainers = [],
    siblingPods: initialSiblingPods = [],
    podContainerMap: initialPodContainerMap = {},
    ownerName: initialOwnerName = '',
    podCreationTime: initialPodCreationTime = '',
    resolveFreshPods,
    tabContext = ''
}: { namespace: any; pod: any; containers?: any; siblingPods?: any; podContainerMap?: any; ownerName?: any; podCreationTime?: any; resolveFreshPods?: any; tabContext?: any }) {
    const { currentContext } = useK8s();
    const { getConfig } = useConfig();
    const [logTarget, setLogTarget] = useState(() => ({
        namespace: initialNamespace,
        pod: initialPod,
        containers: initialContainers,
        siblingPods: initialSiblingPods,
        podContainerMap: initialPodContainerMap,
        ownerName: initialOwnerName,
        podCreationTime: initialPodCreationTime,
    }));

    useEffect(() => {
        setLogTarget({
            namespace: initialNamespace,
            pod: initialPod,
            containers: initialContainers,
            siblingPods: initialSiblingPods,
            podContainerMap: initialPodContainerMap,
            ownerName: initialOwnerName,
            podCreationTime: initialPodCreationTime,
        });
    }, [initialNamespace, initialPod, initialContainers, initialSiblingPods, initialPodContainerMap, initialOwnerName, initialPodCreationTime]);

    const namespace = logTarget.namespace;
    const pod = logTarget.pod;
    const containers = logTarget.containers;
    const siblingPods = logTarget.siblingPods;
    const podContainerMap = logTarget.podContainerMap;
    const ownerName = logTarget.ownerName;
    const podCreationTime = logTarget.podCreationTime;

    // Helper to safely get config with validation and fallback
    const getSafeConfig = useCallback((path: any, defaultValue: any, validator: any) => {
        try {
            const value = getConfig(path);
            if (value === undefined || value === null) return defaultValue;
            if (validator && !validator(value)) {
                console.error(`Invalid config value for ${path}:`, value, '- using default:', defaultValue);
                return defaultValue;
            }
            return value;
        } catch (e: any) {
            console.error(`Error reading config ${path}:`, e, '- using default:', defaultValue);
            return defaultValue;
        }
    }, [getConfig]);

    // UI state
    // Always default to the specific pod that was requested
    const [selectedPod, setSelectedPod] = useState(initialPod);
    // Default to first container for the selected pod
    const [selectedContainer, setSelectedContainer] = useState(containers[0] || '');
    const [wrapLines, setWrapLines] = useState(() => getSafeConfig('logs.lineWrap', true, (v: any) => typeof v === 'boolean'));
    const [showTimestamps, setShowTimestamps] = useState(() => getSafeConfig('logs.showTimestamps', false, (v: any) => typeof v === 'boolean'));
    const [showPrevious, setShowPrevious] = useState(false);
    const [showTimeModal, setShowTimeModal] = useState(false);
    const [sinceTime, setSinceTime] = useState('');
    const initialPosition = getSafeConfig('logs.position', 'end', (v: any) => ['start', 'end', 'all'].includes(v));
    const [viewMode, setViewMode] = useState(initialPosition === 'all' ? 'start' : initialPosition);
    const [autoFollow, setAutoFollow] = useState(true);
    const [downloading, setDownloading] = useState(false);
    const [downloadingBundle, setDownloadingBundle] = useState(false);

    const showDebugDownload = getConfig('debug.showLogSourceMarkers');

    const virtuosoRef = useRef<any>(null);
    const logsContainerRef = useRef<HTMLDivElement>(null);
    const isAtBottomRef = useRef(true);
    // Tracks the underlying-buffer position of the current selection so copy
    // can synthesize text from stream.logs even after Virtuoso unmounts lines.
    const selectionStateRef = useRef<{
        anchor: { idx: number; offset: number } | null;
        focus: { idx: number; offset: number } | null;
    }>({ anchor: null, focus: null });
    // True while a mouse button is held. Browser selectionchange events that
    // fire outside of this window (e.g. scroll-driven node detach/reattach)
    // are ignored so they cannot clobber the frozen selection state.
    const isMouseDownRef = useRef(false);

    // Check if this tab is stale
    const isStale = tabContext && tabContext !== currentContext;

    // Get current containers for the selected pod (or first pod if "All Pods")
    const currentContainers = selectedPod === ALL_PODS
        ? ((podContainerMap as Record<string, any>)[siblingPods[0]] || containers)
        : ((podContainerMap as Record<string, any>)[selectedPod] || containers);

    const resolveFreshLogTarget = useCallback(async ({ pod: requestedPod, container: requestedContainer }: any) => {
        if (!resolveFreshPods) return null;

        const fresh = await resolveFreshPods();
        if (!fresh) return null;

        const freshSiblingPods = fresh.siblingPods || [];
        const freshPodContainerMap = fresh.podContainerMap || {};
        const nextPod = requestedPod === ALL_PODS
            ? ALL_PODS
            : freshSiblingPods.includes(requestedPod)
            ? requestedPod
            : fresh.pod;
        const nextContainers = nextPod === ALL_PODS
            ? (freshPodContainerMap[freshSiblingPods[0]] || fresh.containers || [])
            : (freshPodContainerMap[nextPod] || fresh.containers || []);
        const nextContainer = requestedContainer === ALL_CONTAINERS
            ? ALL_CONTAINERS
            : nextContainers.includes(requestedContainer)
            ? requestedContainer
            : (nextContainers[0] || '');

        setLogTarget({
            namespace: fresh.namespace,
            pod: fresh.pod,
            containers: fresh.containers || [],
            siblingPods: freshSiblingPods,
            podContainerMap: freshPodContainerMap,
            ownerName: fresh.ownerName || '',
            podCreationTime: fresh.podCreationTime || '',
        });
        setSelectedPod(nextPod);
        setSelectedContainer(nextContainer);

        return {
            namespace: fresh.namespace,
            pod: nextPod,
            container: nextContainer,
            containers: nextContainers,
            siblingPods: freshSiblingPods,
            podContainerMap: freshPodContainerMap,
        };
    }, [resolveFreshPods]);

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
        currentContext,
        resolveFreshLogTarget
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
        const handleKeyDown = (e: any) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'r') {
                if (namespace && selectedPod) {
                    stream.fetchLogs({ refreshTarget: true });
                }
            }
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [namespace, selectedPod, selectedContainer, showPrevious, sinceTime, viewMode, stream.fetchLogs]);

    // Paint a precise selection overlay on each mounted line within the
    // recorded buffer range. Native ::selection is hidden via CSS because
    // Virtuoso recycles DOM nodes, making the browser's selection rectangle
    // slide to whichever content currently occupies those nodes. We rebuild
    // a DOM Range against the live content span and lay <div>s over each
    // client rect, so wrap/scroll/recycling all stay visually correct.
    const applySelectionHighlight = useCallback(() => {
        const container = logsContainerRef.current;
        if (!container) return;
        container.querySelectorAll('.log-selection-overlay').forEach(el => el.remove());

        const a = selectionStateRef.current.anchor;
        const f = selectionStateRef.current.focus;
        if (!a || !f) return;
        if (a.idx === f.idx && a.offset === f.offset) return;

        const forward = a.idx < f.idx || (a.idx === f.idx && a.offset <= f.offset);
        const start = forward ? a : f;
        const end = forward ? f : a;

        const rangeForChars = (root: HTMLElement, from: number, to: number): Range | null => {
            const range = document.createRange();
            let counted = 0;
            let startSet = false;
            const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
            let node: Node | null;
            while ((node = walker.nextNode())) {
                const len = (node.textContent ?? '').length;
                const next = counted + len;
                if (!startSet && from <= next) {
                    range.setStart(node, Math.max(0, from - counted));
                    startSet = true;
                }
                if (startSet && to <= next) {
                    range.setEnd(node, Math.max(0, to - counted));
                    return range;
                }
                counted = next;
            }
            if (startSet) {
                range.setEnd(root, root.childNodes.length);
                return range;
            }
            return null;
        };

        container.querySelectorAll<HTMLElement>('[data-log-idx]').forEach(lineEl => {
            const idx = parseInt(lineEl.dataset.logIdx as string, 10);
            if (idx < start.idx || idx > end.idx) return;
            const contentSpan = lineEl.querySelector('[data-log-content]') as HTMLElement | null;
            if (!contentSpan) return;
            const lineLen = contentSpan.textContent?.length ?? 0;
            const fromChar = idx === start.idx ? start.offset : 0;
            const toChar = idx === end.idx ? end.offset : lineLen;
            if (toChar <= fromChar) return;
            const range = rangeForChars(contentSpan, fromChar, toChar);
            if (!range) return;
            const rects = range.getClientRects();
            const lineRect = lineEl.getBoundingClientRect();
            for (let i = 0; i < rects.length; i++) {
                const rect = rects[i];
                if (rect.width === 0 || rect.height === 0) continue;
                const overlay = document.createElement('div');
                overlay.className = 'log-selection-overlay';
                overlay.style.left = (rect.left - lineRect.left) + 'px';
                overlay.style.top = (rect.top - lineRect.top) + 'px';
                overlay.style.width = rect.width + 'px';
                overlay.style.height = rect.height + 'px';
                lineEl.appendChild(overlay);
            }
        });
    }, []);

    // Re-apply the selection overlay whenever Virtuoso swaps content into a
    // recycled line div. itemsRendered runs in a useEffect (after paint), so a
    // stale overlay would flash for a frame; MutationObserver fires in a
    // microtask before the next paint, eliminating the flash. We filter our
    // own overlay add/remove mutations to avoid feedback loops.
    useEffect(() => {
        const container = logsContainerRef.current;
        if (!container) return;
        let rafId: number | null = null;
        const schedule = () => {
            if (rafId !== null) return;
            rafId = requestAnimationFrame(() => {
                rafId = null;
                applySelectionHighlight();
            });
        };
        const isOurOverlayMutation = (m: MutationRecord) => {
            if (m.type !== 'childList') return false;
            const onlyOverlay = (nodes: NodeList) => {
                for (let i = 0; i < nodes.length; i++) {
                    const n = nodes[i];
                    if (!(n instanceof Element) || !n.classList.contains('log-selection-overlay')) return false;
                }
                return true;
            };
            return onlyOverlay(m.addedNodes) && onlyOverlay(m.removedNodes);
        };
        const observer = new MutationObserver((mutations) => {
            for (const m of mutations) {
                if (isOurOverlayMutation(m)) continue;
                schedule();
                return;
            }
        });
        observer.observe(container, {
            childList: true,
            subtree: true,
            attributes: true,
            attributeFilter: ['data-log-idx']
        });
        return () => {
            observer.disconnect();
            if (rafId !== null) cancelAnimationFrame(rafId);
        };
    }, [applySelectionHighlight]);

    // Track the underlying-buffer position of the current selection. Virtuoso
    // unmounts off-screen lines, so a native copy would only include text that
    // happens to be in the DOM. We record (idx, offset) into stream.logs on
    // every selectionchange (skipping ends whose line is no longer mounted) and
    // synthesize the copy text from the buffer below.
    useEffect(() => {
        const findLogLine = (node: Node | null): HTMLElement | null => {
            let el: HTMLElement | null = node && node.nodeType === 1
                ? (node as HTMLElement)
                : node?.parentElement ?? null;
            const container = logsContainerRef.current;
            while (el && el !== document.body) {
                if (el.dataset && el.dataset.logIdx !== undefined) return el;
                if (container && el === container) return null;
                el = el.parentElement;
            }
            return null;
        };

        const computeOffset = (lineEl: HTMLElement, container: Node, offset: number) => {
            const span = lineEl.querySelector('[data-log-content]');
            if (!span) return 0;
            try {
                const range = document.createRange();
                range.setStart(span, 0);
                range.setEnd(container, offset);
                return range.toString().length;
            } catch {
                return 0;
            }
        };

        const handleSelectionChange = () => {
            const sel = document.getSelection();
            if (!sel || sel.rangeCount === 0) return;

            // Freeze the selection state once the user releases the mouse —
            // any subsequent selectionchange is browser-driven (scroll detaches
            // nodes, browser collapses/re-anchors the range) and would corrupt
            // our recorded position. Allow the first update though, so we can
            // pick up the initial selection.
            const hasState = !!(selectionStateRef.current.anchor || selectionStateRef.current.focus);
            if (!isMouseDownRef.current && hasState) return;

            const anchorLineEl = findLogLine(sel.anchorNode);
            const focusLineEl = findLogLine(sel.focusNode);

            if (anchorLineEl) {
                selectionStateRef.current.anchor = {
                    idx: parseInt(anchorLineEl.dataset.logIdx as string, 10),
                    offset: computeOffset(anchorLineEl, sel.anchorNode!, sel.anchorOffset)
                };
            }
            if (focusLineEl) {
                selectionStateRef.current.focus = {
                    idx: parseInt(focusLineEl.dataset.logIdx as string, 10),
                    offset: computeOffset(focusLineEl, sel.focusNode!, sel.focusOffset)
                };
            }
            applySelectionHighlight();
        };

        const handleMouseDown = (e: MouseEvent) => {
            isMouseDownRef.current = true;
            // Click without shift inside our container starts a new selection;
            // clear stored state so the freeze gate doesn't suppress the first
            // selectionchange of the new interaction.
            const target = e.target as Node | null;
            if (logsContainerRef.current?.contains(target) && !e.shiftKey) {
                selectionStateRef.current = { anchor: null, focus: null };
            }
        };
        const handleMouseUp = () => { isMouseDownRef.current = false; };

        document.addEventListener('selectionchange', handleSelectionChange);
        document.addEventListener('mousedown', handleMouseDown);
        document.addEventListener('mouseup', handleMouseUp);
        return () => {
            document.removeEventListener('selectionchange', handleSelectionChange);
            document.removeEventListener('mousedown', handleMouseDown);
            document.removeEventListener('mouseup', handleMouseUp);
        };
    }, [applySelectionHighlight]);

    // Intercept copy so we pull text from stream.logs (the full underlying
    // buffer) rather than the partial DOM, using the offsets recorded above.
    useEffect(() => {
        const handleCopy = (e: ClipboardEvent) => {
            const sel = document.getSelection();
            if (!sel || sel.rangeCount === 0) return;
            const container = logsContainerRef.current;
            if (!container) return;
            const inContainer = container.contains(sel.anchorNode) || container.contains(sel.focusNode);
            if (!inContainer) return;

            const state = selectionStateRef.current;
            if (!state.anchor || !state.focus) return;

            const a = state.anchor;
            const f = state.focus;
            const forward = a.idx < f.idx || (a.idx === f.idx && a.offset <= f.offset);
            const start = forward ? a : f;
            const end = forward ? f : a;
            if (start.idx === end.idx && start.offset === end.offset) return;

            const parsePrefixesNow = selectedPod === ALL_PODS || selectedContainer === ALL_CONTAINERS;
            const prefixRe = /^\[([a-z0-9][a-z0-9\-.]*(\/[a-z0-9][a-z0-9\-.]*)?)\]\s*/;
            const logs = stream.logs;
            const parts: string[] = [];
            for (let i = start.idx; i <= end.idx; i++) {
                const entry = logs[i];
                if (!entry) { parts.push(''); continue; }
                let content: string = entry.content || '';
                if (parsePrefixesNow) {
                    const m = content.match(prefixRe);
                    if (m) content = content.slice(m[0].length);
                }
                let text = stripAnsiCodes(content);
                if (i === start.idx && i === end.idx) {
                    text = text.slice(start.offset, end.offset);
                } else if (i === start.idx) {
                    text = text.slice(start.offset);
                } else if (i === end.idx) {
                    text = text.slice(0, end.offset);
                }
                parts.push(text);
            }

            e.clipboardData?.setData('text/plain', parts.join('\n'));
            e.preventDefault();
        };

        document.addEventListener('copy', handleCopy);
        return () => document.removeEventListener('copy', handleCopy);
    }, [stream.logs, selectedPod, selectedContainer]);


    // Virtuoso callbacks
    const handleAtBottomStateChange = useCallback((atBottom: any) => {
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

    const followOutput = useCallback((isAtBottom: any) => {
        if (isFollowing && isAtBottom) return 'smooth';
        return false;
    }, [isFollowing]);

    // Scroll helpers
    const scrollToTop = useCallback(() => {
        isAtBottomRef.current = false;
        (virtuosoRef as any).current?.scrollToIndex({ index: 0, behavior: 'auto' });
    }, []);

    const scrollToBottom = useCallback(() => {
        isAtBottomRef.current = true;
        (virtuosoRef as any).current?.scrollToIndex({ index: 'LAST', behavior: 'auto' });
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

    const handleTimeApply = (time: any) => {
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
                const podPairs = siblingPods.map((podName: any) => ({
                    podName,
                    containerNames: isAllContainers
                        ? ((podContainerMap as Record<string, any>)[podName] || containers)
                        : (selectedContainer ? [selectedContainer] : [])
                }));
                allLogs = await GetAllPodsLogsAll(namespace, podPairs, isAllContainers, showTimestamps, showPrevious);
            } else if (isAllContainers) {
                allLogs = await GetAllContainersLogsAll(namespace, selectedPod, currentContainers, showTimestamps, showPrevious);
            } else {
                allLogs = await GetAllPodLogs(namespace, selectedPod, selectedContainer, showTimestamps, showPrevious);
            }
            await SavePodLogs(allLogs, filename);
        } catch (err: any) {
            console.error('Failed to save logs:', err);
            stream.setFetchError(`Failed to download logs: ${err}`);
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
                const podContainers = (podContainerMap as Record<string, any>)[podName] || containers;
                for (const containerName of podContainers) {
                    try {
                        const logs = await GetAllPodLogs(namespace, podName, containerName, showTimestamps, showPrevious);
                        entries.push({ podName, containerName, logs: logs || '' });
                    } catch (err: any) {
                        entries.push({ podName, containerName, logs: `Error fetching logs: ${err}` });
                    }
                }
            }
            await SaveLogsBundle(entries, filename);
        } catch (err: any) {
            console.error('Failed to save logs bundle:', err);
            stream.setFetchError(`Failed to download logs bundle: ${err}`);
        } finally {
            setDownloadingBundle(false);
        }
    };

    const downloadVisibleLogs = async () => {
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `${selectedPod}-${timestamp}.log`;
        try {
            await SavePodLogs(logsToVisibleString(stream.logs, showTimestamps), filename);
        } catch (err: any) {
            console.error('Failed to save visible logs:', err);
        }
    };

    const downloadDebugLogs = async () => {
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `DEBUG-${selectedPod}-${timestamp}.log`;
        try {
            await SavePodLogs(logsToDebugString(stream.logs), filename);
        } catch (err: any) {
            console.error('Failed to save debug logs:', err);
        }
    };

    // Render log item
    const parsePrefixes = selectedPod === ALL_PODS || selectedContainer === ALL_CONTAINERS;

    const renderLogItem = useCallback((index: any) => {
        const entry = search.displayLogs[index];
        if (!entry) return null;

        return (
            <LogLine
                entry={entry}
                showTimestamps={showTimestamps}
                searchTerm={search.searchTerm}
                searchRegex={search.searchRegex}
                wrapLines={wrapLines}
                parsePrefixes={parsePrefixes}
            />
        );
    }, [search.displayLogs, showTimestamps, search.searchTerm, search.searchRegex, wrapLines, parsePrefixes]);

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

            {/* Error/Warning Banner — single slot, fetchError takes precedence */}
            {!isStale && (stream.fetchError || stream.streamDisconnected) && (
                <div className="flex items-center justify-between px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <div className="flex items-center gap-2">
                        <ExclamationTriangleIcon className="h-5 w-5" />
                        <span className="text-sm">{stream.fetchError || stream.disconnectReason || 'Stream disconnected'}</span>
                    </div>
                    <div className="flex items-center gap-2">
                        {stream.fetchError && (
                            <button
                                onClick={stream.clearFetchError}
                                className="text-xs text-amber-400 hover:text-amber-300 px-2"
                            >
                                Dismiss
                            </button>
                        )}
                        <button
                            onClick={() => stream.fetchLogs({ refreshTarget: true })}
                            disabled={stream.loading}
                            className="flex items-center gap-1.5 px-3 py-1 text-xs font-medium bg-amber-600 text-white rounded hover:bg-amber-500 transition-colors disabled:opacity-50"
                        >
                            <ArrowPathIcon className={`h-4 w-4 ${stream.loading ? 'animate-spin' : ''}`} />
                            Refresh
                        </button>
                    </div>
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
                                getOptionLabel={(opt: any) => opt === ALL_PODS ? 'All Pods' : opt}
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
                                    getOptionLabel={(opt: any) => opt === ALL_CONTAINERS ? 'All Containers' : opt}
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
                            disabled={!!stream.fetchError}
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
                            disabled={stream.loading || stream.loadingAll || stream.isAllLoaded || !!stream.fetchError}
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
                            disabled={stream.logs.length === 0 || stream.loading || downloading || !!stream.fetchError}
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
                                disabled={stream.loading || downloadingBundle || !!stream.fetchError}
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
                            onChange={(e: any) => search.setSearchInput(e.target.value)}
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
            <div ref={logsContainerRef} className="flex-1 overflow-hidden text-gray-300 font-mono text-xs" data-selectable-region data-log-region tabIndex={-1}>
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
                        increaseViewportBy={{ top: 0, bottom: 0 }}
                        itemsRendered={applySelectionHighlight}
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
