import React, { useState, useCallback, useMemo, useRef, useEffect } from 'react';
import { Virtuoso } from 'react-virtuoso';
import { GetMultiPodLogs, ListPods } from '../../../wailsjs/go/main/App';
import { useK8s } from '../../context';
import SearchSelect from '../../components/shared/SearchSelect';
import Tooltip from '../../components/shared/Tooltip';
import { converter, normalizeAnsiCodes, stripAnsiCodes } from '../../components/shared/log-viewer/logUtils';
import {
    MagnifyingGlassIcon,
    ArrowPathIcon,
    XMarkIcon,
    DocumentTextIcon,
    ArrowDownTrayIcon,
    TagIcon,
    ChevronDoubleDownIcon,
    ChevronDoubleUpIcon
} from '@heroicons/react/24/outline';

const TAIL_OPTIONS = [
    { value: 50, label: '50' },
    { value: 100, label: '100' },
    { value: 500, label: '500' },
    { value: 1000, label: '1000' }
];

const SINCE_OPTIONS = [
    { value: 60, label: '1 min' },
    { value: 300, label: '5 min' },
    { value: 600, label: '10 min' },
    { value: 1800, label: '30 min' },
    { value: 3600, label: '1 hour' }
];

function formatTimestamp(timestamp) {
    const date = new Date(timestamp);
    return date.toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        fractionalSecondDigits: 3,
        hour12: false
    });
}

function LogLine({ entry, showPodName = true, showTimestamp = true }) {
    // Convert ANSI codes to HTML for colored output
    const htmlMessage = useMemo(() => {
        const normalized = normalizeAnsiCodes(entry.message || '');
        return converter.toHtml(normalized);
    }, [entry.message]);

    return (
        <div className="flex font-mono text-xs hover:bg-surface-light px-2 py-0.5">
            {showTimestamp && (
                <span className="text-gray-500 mr-3 flex-shrink-0">
                    {formatTimestamp(entry.timestamp)}
                </span>
            )}
            {showPodName && (
                <span
                    className="mr-3 flex-shrink-0 font-medium"
                    style={{ color: entry.color }}
                >
                    [{entry.podName}/{entry.container}]
                </span>
            )}
            <span
                className="text-text whitespace-pre-wrap break-all"
                dangerouslySetInnerHTML={{ __html: htmlMessage }}
            />
        </div>
    );
}

export default function MultiLogViewer({
    initialNamespace = '',
    initialPodNames = [],
    initialLabelSelector = {},
    onClose
}) {
    const { currentNamespace, currentContext, namespaces } = useK8s();
    const virtuosoRef = useRef(null);

    // Sanitize namespace - don't use '*' (All Namespaces marker)
    const effectiveCurrentNamespace = (currentNamespace && currentNamespace !== '*') ? currentNamespace : 'default';
    const effectiveInitialNamespace = (initialNamespace && initialNamespace !== '*') ? initialNamespace : '';

    // Form state - namespace is now an array for multi-select (use '*' for all namespaces)
    const [selectedNamespaces, setSelectedNamespaces] = useState(
        effectiveInitialNamespace ? [effectiveInitialNamespace] : [effectiveCurrentNamespace]
    );
    const [selectorMode, setSelectorMode] = useState(
        Object.keys(initialLabelSelector).length > 0 ? 'label' : 'pods'
    );
    const [selectedPods, setSelectedPods] = useState(initialPodNames);
    const [labelSelector, setLabelSelector] = useState(
        Object.entries(initialLabelSelector).map(([k, v]) => `${k}=${v}`).join(', ')
    );
    const [container, setContainer] = useState('');
    const [tailLines, setTailLines] = useState(100);
    const [sinceSeconds, setSinceSeconds] = useState(300);

    // Available pods for selection
    const [availablePods, setAvailablePods] = useState([]);
    const [loadingPods, setLoadingPods] = useState(false);

    // Display state
    const [entries, setEntries] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [searchQuery, setSearchQuery] = useState('');
    const [showTimestamps, setShowTimestamps] = useState(true);
    const [showPodNames, setShowPodNames] = useState(true);
    const [filterPod, setFilterPod] = useState('');

    // Compute effective namespace for API calls
    // '*' means all namespaces (fetch with empty string), otherwise use the selected namespace(s)
    const isAllNamespaces = selectedNamespaces.includes('*') || selectedNamespaces.length === 0;
    const effectiveNamespace = isAllNamespaces ? '' : selectedNamespaces[0];

    // Fetch available pods when namespace changes
    // Empty namespace = all namespaces (show namespace-prefixed pod names)
    useEffect(() => {
        setLoadingPods(true);
        setSelectedPods([]); // Clear selection when namespace changes
        const requestId = `multi-log-pods-${Date.now()}`;
        // Empty string fetches from all namespaces
        ListPods(requestId, effectiveNamespace)
            .then(pods => {
                const podItems = (pods || []).map(p => {
                    const name = p.metadata?.name;
                    const ns = p.metadata?.namespace;
                    // When showing all namespaces, prefix pod name with namespace
                    return isAllNamespaces ? `${ns}/${name}` : name;
                }).filter(Boolean).sort();
                setAvailablePods(podItems);
            })
            .catch(err => {
                console.error('Failed to list pods:', err);
                setAvailablePods([]);
            })
            .finally(() => setLoadingPods(false));
    }, [effectiveNamespace, isAllNamespaces, currentContext]);

    // Auto-fetch logs if opened with initial parameters
    const autoFetchDone = useRef(false);
    useEffect(() => {
        if (autoFetchDone.current) return;

        // Only auto-fetch if we have initial parameters
        const hasInitialPods = initialPodNames && initialPodNames.length > 0;
        const hasInitialLabels = initialLabelSelector && Object.keys(initialLabelSelector).length > 0;

        if (hasInitialPods || hasInitialLabels) {
            autoFetchDone.current = true;
            // Small delay to let the component state settle, then trigger fetch
            const timer = setTimeout(() => {
                // Inline fetch logic to avoid circular dependency
                const doFetch = async () => {
                    setLoading(true);
                    setError(null);
                    try {
                        const labels = hasInitialLabels ? initialLabelSelector : {};
                        const pods = hasInitialPods ? initialPodNames : [];

                        console.log('[MultiLogViewer] Auto-fetch:', { effectiveNamespace, labels, pods });

                        const result = await GetMultiPodLogs(
                            effectiveNamespace,
                            labels,
                            pods,
                            '', // container
                            tailLines,
                            sinceSeconds
                        );
                        console.log('[MultiLogViewer] Auto-fetch result:', result?.length || 0, 'entries');
                        setEntries(result || []);
                    } catch (err) {
                        setError(err.message || 'Failed to fetch logs');
                        setEntries([]);
                    } finally {
                        setLoading(false);
                    }
                };
                doFetch();
            }, 200);
            return () => clearTimeout(timer);
        }
    }, []); // Empty deps - only run on mount

    // Parse label selector string to object
    const parseLabelSelector = useCallback((str) => {
        if (!str.trim()) return {};
        const result = {};
        str.split(',').forEach(pair => {
            const [key, value] = pair.trim().split('=').map(s => s.trim());
            if (key && value !== undefined) {
                result[key] = value;
            }
        });
        return result;
    }, []);

    // Fetch logs
    const fetchLogs = useCallback(async () => {
        setLoading(true);
        setError(null);

        try {
            const labels = selectorMode === 'label' ? parseLabelSelector(labelSelector) : {};
            // Convert '*' (All Pods marker) to actual pod list
            let pods = selectorMode === 'pods' ? selectedPods : [];
            if (pods.includes('*')) {
                pods = availablePods;
            }

            console.log('[MultiLogViewer] fetchLogs called:', {
                effectiveNamespace,
                isAllNamespaces,
                selectorMode,
                labels,
                pods,
                container,
                tailLines,
                sinceSeconds
            });

            if (selectorMode === 'label' && Object.keys(labels).length === 0) {
                setError('Please provide at least one label selector');
                setLoading(false);
                return;
            }
            if (selectorMode === 'pods' && pods.length === 0) {
                setError('Please select at least one pod');
                setLoading(false);
                return;
            }

            let result = [];

            // When in "All Namespaces" mode, pods are prefixed with namespace/podname
            // Group by namespace and make separate API calls
            if (isAllNamespaces && selectorMode === 'pods') {
                const podsByNamespace = {};
                for (const pod of pods) {
                    const slashIdx = pod.indexOf('/');
                    if (slashIdx > 0) {
                        const ns = pod.substring(0, slashIdx);
                        const name = pod.substring(slashIdx + 1);
                        if (!podsByNamespace[ns]) podsByNamespace[ns] = [];
                        podsByNamespace[ns].push(name);
                    }
                }

                // Fetch logs from each namespace in parallel
                const promises = Object.entries(podsByNamespace).map(([ns, podNames]) =>
                    GetMultiPodLogs(ns, {}, podNames, container, tailLines, sinceSeconds)
                        .catch(err => {
                            console.error(`[MultiLogViewer] Failed to fetch logs from ${ns}:`, err);
                            return [];
                        })
                );
                const results = await Promise.all(promises);
                result = results.flat();
            } else {
                // Single namespace mode
                result = await GetMultiPodLogs(
                    effectiveNamespace,
                    labels,
                    pods,
                    container,
                    tailLines,
                    sinceSeconds
                );
            }

            console.log('[MultiLogViewer] GetMultiPodLogs result:', result?.length || 0, 'entries');
            setEntries(result || []);

            // Auto-scroll to bottom
            if (virtuosoRef.current && result?.length > 0) {
                setTimeout(() => {
                    virtuosoRef.current?.scrollToIndex({
                        index: result.length - 1,
                        behavior: 'smooth'
                    });
                }, 100);
            }
        } catch (err) {
            setError(err.message || 'Failed to fetch logs');
            setEntries([]);
        } finally {
            setLoading(false);
        }
    }, [effectiveNamespace, isAllNamespaces, selectorMode, labelSelector, selectedPods, container, tailLines, sinceSeconds, parseLabelSelector, availablePods]);

    // Get unique pods for filter dropdown
    const uniquePods = useMemo(() => {
        const pods = new Set();
        entries.forEach(e => pods.add(e.podName));
        return Array.from(pods).sort();
    }, [entries]);

    // Filter entries
    const filteredEntries = useMemo(() => {
        return entries.filter(entry => {
            if (filterPod && entry.podName !== filterPod) return false;
            if (searchQuery) {
                const query = searchQuery.toLowerCase();
                // Strip ANSI codes before searching so matches work across color boundaries
                const plainMessage = stripAnsiCodes(entry.message || '').toLowerCase();
                if (!plainMessage.includes(query)) return false;
            }
            return true;
        });
    }, [entries, filterPod, searchQuery]);

    // Download logs
    const downloadLogs = useCallback(() => {
        const content = filteredEntries.map(e => {
            const ts = new Date(e.timestamp).toISOString();
            return `${ts} [${e.podName}/${e.container}] ${e.message}`;
        }).join('\n');

        const blob = new Blob([content], { type: 'text/plain' });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = `multi-pod-logs-${new Date().toISOString().slice(0, 19).replace(/:/g, '-')}.log`;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    }, [filteredEntries]);

    return (
        <div className="h-full flex flex-col bg-background text-text">
            {/* Header */}
            <div className="flex-shrink-0 border-b border-border p-4">
                <div className="flex items-center justify-between mb-4">
                    <h2 className="text-lg font-semibold flex items-center gap-2">
                        <DocumentTextIcon className="h-5 w-5 text-green-400" />
                        Multi-Pod Log Viewer
                    </h2>
                    {onClose && (
                        <button
                            onClick={onClose}
                            className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                        >
                            <XMarkIcon className="h-5 w-5" />
                        </button>
                    )}
                </div>

                {/* Selector Mode Toggle */}
                <div className="flex gap-2 mb-3">
                    <button
                        onClick={() => setSelectorMode('pods')}
                        className={`px-3 py-1.5 rounded text-sm flex items-center gap-1.5 transition-colors ${
                            selectorMode === 'pods'
                                ? 'bg-primary text-white'
                                : 'bg-surface text-gray-300 hover:bg-surface-light'
                        }`}
                    >
                        <DocumentTextIcon className="h-4 w-4" />
                        Select Pods
                    </button>
                    <button
                        onClick={() => setSelectorMode('label')}
                        className={`px-3 py-1.5 rounded text-sm flex items-center gap-1.5 transition-colors ${
                            selectorMode === 'label'
                                ? 'bg-primary text-white'
                                : 'bg-surface text-gray-300 hover:bg-surface-light'
                        }`}
                    >
                        <TagIcon className="h-4 w-4" />
                        Label Selector
                    </button>
                </div>

                {/* Input Form */}
                <div className="grid grid-cols-1 md:grid-cols-3 gap-3 mb-3">
                    <div>
                        <label className="block text-xs text-gray-400 mb-1">Namespace</label>
                        <SearchSelect
                            options={namespaces}
                            value={selectedNamespaces}
                            onChange={setSelectedNamespaces}
                            placeholder="Select namespace..."
                            multiSelect={true}
                        />
                    </div>
                    {selectorMode === 'pods' ? (
                        <div className="md:col-span-2">
                            <label className="block text-xs text-gray-400 mb-1">
                                Pods {loadingPods && <span className="text-gray-500">(loading...)</span>}
                            </label>
                            <SearchSelect
                                options={availablePods}
                                value={selectedPods}
                                onChange={setSelectedPods}
                                placeholder="Select pods..."
                                multiSelect={true}
                                disabled={loadingPods}
                                multiSelectLabels={{
                                    all: 'All Pods',
                                    count: (n) => `${n} pods selected`
                                }}
                            />
                        </div>
                    ) : (
                        <div className="md:col-span-2">
                            <label className="block text-xs text-gray-400 mb-1">
                                Label Selector (e.g., app=nginx, tier=frontend)
                            </label>
                            <input
                                type="text"
                                value={labelSelector}
                                onChange={(e) => setLabelSelector(e.target.value)}
                                className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                                placeholder="app=nginx"
                                autoComplete="off"
                            />
                        </div>
                    )}
                </div>

                <div className="flex items-center gap-3 flex-wrap">
                    <div className="flex items-center gap-2">
                        <label className="text-xs text-gray-400">Container:</label>
                        <input
                            type="text"
                            value={container}
                            onChange={(e) => setContainer(e.target.value)}
                            className="w-24 px-2 py-1 bg-background border border-border rounded text-xs text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                            placeholder="all"
                            autoComplete="off"
                        />
                    </div>
                    <div className="flex items-center gap-2">
                        <label className="text-xs text-gray-400">Tail:</label>
                        <div className="w-20">
                            <SearchSelect
                                options={TAIL_OPTIONS}
                                value={tailLines}
                                onChange={setTailLines}
                                placeholder="Tail"
                                getOptionValue={(t) => t.value}
                                getOptionLabel={(t) => t.label}
                                preserveOrder={true}
                                searchable={false}
                            />
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <label className="text-xs text-gray-400">Since:</label>
                        <div className="w-24">
                            <SearchSelect
                                options={SINCE_OPTIONS}
                                value={sinceSeconds}
                                onChange={setSinceSeconds}
                                placeholder="Since"
                                getOptionValue={(s) => s.value}
                                getOptionLabel={(s) => s.label}
                                preserveOrder={true}
                                searchable={false}
                            />
                        </div>
                    </div>
                    <button
                        onClick={fetchLogs}
                        disabled={loading}
                        className="px-4 py-1.5 bg-primary hover:bg-primary/80 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium text-white flex items-center gap-2 transition-colors"
                    >
                        {loading ? (
                            <ArrowPathIcon className="h-4 w-4 animate-spin" />
                        ) : (
                            <MagnifyingGlassIcon className="h-4 w-4" />
                        )}
                        Fetch Logs
                    </button>
                </div>
            </div>

            {/* Error Display */}
            {error && (
                <div className="flex-shrink-0 p-4 bg-red-900/20 border-b border-red-800/50 text-red-400 text-sm">
                    {error}
                </div>
            )}

            {/* Toolbar */}
            {entries.length > 0 && (
                <div className="flex-shrink-0 p-2 border-b border-border bg-surface flex items-center justify-between gap-4">
                    <div className="flex items-center gap-3">
                        <span className="text-xs text-gray-400">
                            {filteredEntries.length} / {entries.length} lines
                        </span>
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-400">Pods:</span>
                            {uniquePods.map((pod, idx) => (
                                <button
                                    key={pod}
                                    onClick={() => setFilterPod(filterPod === pod ? '' : pod)}
                                    className={`text-xs px-2 py-0.5 rounded transition-colors ${
                                        filterPod === pod
                                            ? 'bg-primary text-white'
                                            : 'bg-surface-light hover:bg-white/10 text-gray-300'
                                    }`}
                                    style={{
                                        borderLeft: `3px solid ${
                                            ['#3B82F6', '#10B981', '#F59E0B', '#EF4444', '#8B5CF6', '#EC4899', '#06B6D4', '#84CC16', '#F97316', '#6366F1'][idx % 10]
                                        }`
                                    }}
                                >
                                    {pod}
                                </button>
                            ))}
                        </div>
                    </div>

                    <div className="flex items-center gap-2">
                        <div className="relative">
                            <MagnifyingGlassIcon className="h-3.5 w-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-gray-500" />
                            <input
                                type="text"
                                value={searchQuery}
                                onChange={(e) => setSearchQuery(e.target.value)}
                                placeholder="Filter..."
                                className="w-36 pl-7 pr-2 py-1 bg-surface border border-border rounded text-xs text-text focus:outline-none focus:border-primary"
                                autoComplete="off"
                            />
                        </div>
                        <label className="flex items-center gap-1 text-xs text-gray-400 cursor-pointer">
                            <input
                                type="checkbox"
                                checked={showTimestamps}
                                onChange={(e) => setShowTimestamps(e.target.checked)}
                                className="rounded bg-surface border-border"
                            />
                            Timestamps
                        </label>
                        <label className="flex items-center gap-1 text-xs text-gray-400 cursor-pointer">
                            <input
                                type="checkbox"
                                checked={showPodNames}
                                onChange={(e) => setShowPodNames(e.target.checked)}
                                className="rounded bg-surface border-border"
                            />
                            Pod Names
                        </label>
                        <Tooltip content="Scroll to top">
                            <button
                                onClick={() => virtuosoRef.current?.scrollToIndex({ index: 0 })}
                                className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <ChevronDoubleUpIcon className="h-4 w-4" />
                            </button>
                        </Tooltip>
                        <Tooltip content="Scroll to bottom">
                            <button
                                onClick={() => virtuosoRef.current?.scrollToIndex({ index: filteredEntries.length - 1 })}
                                className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <ChevronDoubleDownIcon className="h-4 w-4" />
                            </button>
                        </Tooltip>
                        <Tooltip content="Download logs">
                            <button
                                onClick={downloadLogs}
                                className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <ArrowDownTrayIcon className="h-4 w-4" />
                            </button>
                        </Tooltip>
                    </div>
                </div>
            )}

            {/* Log Content */}
            <div className="flex-1 min-h-0 overflow-hidden bg-background">
                {loading ? (
                    <div className="flex items-center justify-center h-32">
                        <ArrowPathIcon className="h-8 w-8 text-gray-500 animate-spin" />
                    </div>
                ) : entries.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                        <DocumentTextIcon className="h-12 w-12 mb-2 opacity-50" />
                        <p>Select pods and fetch logs to view them here</p>
                    </div>
                ) : filteredEntries.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                        <MagnifyingGlassIcon className="h-12 w-12 mb-2 opacity-50" />
                        <p>No logs match your filter</p>
                    </div>
                ) : (
                    <Virtuoso
                        ref={virtuosoRef}
                        data={filteredEntries}
                        itemContent={(index, entry) => (
                            <LogLine
                                entry={entry}
                                showPodName={showPodNames}
                                showTimestamp={showTimestamps}
                            />
                        )}
                        style={{ height: '100%' }}
                    />
                )}
            </div>
        </div>
    );
}
