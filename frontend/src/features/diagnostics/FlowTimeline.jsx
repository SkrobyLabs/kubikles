import React, { useState, useCallback, useMemo, useEffect } from 'react';
import { Virtuoso } from 'react-virtuoso';
import {
    GetFlowTimeline,
    ListDeployments,
    ListStatefulSets,
    ListDaemonSets,
    ListPods,
    ListServices,
    ListIngresses,
    ListJobs,
    ListCronJobs
} from '../../../wailsjs/go/main/App';
import { useK8s } from '../../context';
import Tooltip from '../../components/shared/Tooltip';
import SearchSelect from '../../components/shared/SearchSelect';
import {
    MagnifyingGlassIcon,
    ClockIcon,
    ExclamationTriangleIcon,
    ExclamationCircleIcon,
    InformationCircleIcon,
    DocumentTextIcon,
    BellAlertIcon,
    ArrowPathIcon,
    XMarkIcon,
    ChevronDownIcon,
    ChevronRightIcon
} from '@heroicons/react/24/outline';

const RESOURCE_TYPES = [
    { value: 'deployment', label: 'Deployment' },
    { value: 'statefulset', label: 'StatefulSet' },
    { value: 'daemonset', label: 'DaemonSet' },
    { value: 'pod', label: 'Pod' },
    { value: 'service', label: 'Service' },
    { value: 'ingress', label: 'Ingress' },
    { value: 'job', label: 'Job' },
    { value: 'cronjob', label: 'CronJob' }
];

// Helper to fetch resources - handles different function signatures
// Backend has inconsistent signatures: some need (requestId, ns), some just (ns), some (requestId, ctx, ns)
const fetchResourcesByType = async (type, namespace) => {
    const requestId = `flow-timeline-${type}-${Date.now()}`;
    switch (type) {
        case 'deployment':
            return ListDeployments(requestId, namespace);
        case 'statefulset':
            // ListStatefulSets has signature (requestId, contextName, namespace) - pass empty context for current
            return ListStatefulSets(requestId, '', namespace);
        case 'daemonset':
            return ListDaemonSets(requestId, namespace);
        case 'pod':
            return ListPods(requestId, namespace);
        case 'service':
            return ListServices(requestId, namespace);
        case 'ingress':
            // ListIngresses has signature (namespace)
            return ListIngresses(namespace);
        case 'job':
            return ListJobs(requestId, namespace);
        case 'cronjob':
            return ListCronJobs(requestId, namespace);
        default:
            return [];
    }
};

const DURATION_OPTIONS = [
    { value: 5, label: '5 min' },
    { value: 10, label: '10 min' },
    { value: 30, label: '30 min' },
    { value: 60, label: '1 hour' },
    { value: 180, label: '3 hours' }
];

const SEVERITY_OPTIONS = [
    { value: 'all', label: 'All Severity' },
    { value: 'error', label: 'Errors' },
    { value: 'warning', label: 'Warnings' },
    { value: 'info', label: 'Info' }
];

const TYPE_OPTIONS = [
    { value: 'all', label: 'All Types' },
    { value: 'event', label: 'Events' },
    { value: 'log', label: 'Logs' }
];

const SEVERITY_COLORS = {
    error: 'text-red-400 bg-red-900/20 border-l-red-500',
    warning: 'text-amber-400 bg-amber-900/20 border-l-amber-500',
    info: 'text-blue-400 bg-blue-900/20 border-l-blue-500'
};

const SEVERITY_ICONS = {
    error: ExclamationCircleIcon,
    warning: ExclamationTriangleIcon,
    info: InformationCircleIcon
};

const ENTRY_TYPE_ICONS = {
    event: BellAlertIcon,
    log: DocumentTextIcon,
    change: ArrowPathIcon
};

function formatTimestamp(timestamp) {
    const date = new Date(timestamp);
    return date.toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
        second: '2-digit',
        hour12: false
    });
}

function formatRelativeTime(timestamp) {
    const now = new Date();
    const then = new Date(timestamp);
    const diffMs = now - then;
    const diffSec = Math.floor(diffMs / 1000);
    const diffMin = Math.floor(diffSec / 60);
    const diffHour = Math.floor(diffMin / 60);

    if (diffSec < 60) return `${diffSec}s ago`;
    if (diffMin < 60) return `${diffMin}m ago`;
    return `${diffHour}h ${diffMin % 60}m ago`;
}

function TimelineEntry({ entry, expanded, onToggle }) {
    const SeverityIcon = SEVERITY_ICONS[entry.severity] || InformationCircleIcon;
    const TypeIcon = ENTRY_TYPE_ICONS[entry.entryType] || DocumentTextIcon;
    const colorClass = SEVERITY_COLORS[entry.severity] || SEVERITY_COLORS.info;

    return (
        <div className={`border-l-2 pl-4 py-2 mb-2 rounded-r bg-surface ${colorClass}`}>
            <div
                className="flex items-start gap-3 cursor-pointer"
                onClick={onToggle}
            >
                <div className="flex-shrink-0 flex items-center gap-1 mt-0.5">
                    <SeverityIcon className="h-4 w-4" />
                    <TypeIcon className="h-4 w-4 opacity-50" />
                </div>
                <div className="flex-1 min-w-0">
                    <div className="flex items-center gap-2 text-xs text-gray-400 mb-1">
                        <span className="font-mono">{formatTimestamp(entry.timestamp)}</span>
                        <span className="text-gray-600">|</span>
                        <span className="font-medium text-gray-300">{entry.resourceRef}</span>
                        <span className="text-gray-600">|</span>
                        <span className="capitalize">{entry.entryType}</span>
                    </div>
                    <div className="text-sm text-text break-words">
                        {entry.message}
                    </div>
                </div>
                <div className="flex-shrink-0 text-xs text-gray-500">
                    {formatRelativeTime(entry.timestamp)}
                </div>
                {entry.details && (
                    <div className="flex-shrink-0">
                        {expanded ?
                            <ChevronDownIcon className="h-4 w-4 text-gray-500" /> :
                            <ChevronRightIcon className="h-4 w-4 text-gray-500" />
                        }
                    </div>
                )}
            </div>
            {expanded && entry.details && (
                <div className="mt-2 ml-8 p-2 bg-background rounded text-xs font-mono text-gray-400 whitespace-pre-wrap">
                    {entry.details}
                </div>
            )}
        </div>
    );
}

export default function FlowTimeline({
    resourceType = '',
    namespace = '',
    name = '',
    onClose
}) {
    const { currentNamespace, namespaces } = useK8s();
    // Don't use '*' (All Namespaces) as a namespace - fall back to 'default'
    const effectiveNamespace = (currentNamespace && currentNamespace !== '*') ? currentNamespace : 'default';
    const [formData, setFormData] = useState({
        resourceType: resourceType || 'deployment',
        namespace: namespace || effectiveNamespace,
        name: name || ''
    });
    const [durationMinutes, setDurationMinutes] = useState(10);
    const [includeLogs, setIncludeLogs] = useState(true);
    const [entries, setEntries] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [expandedEntries, setExpandedEntries] = useState(new Set());
    const [severityFilter, setSeverityFilter] = useState('all');
    const [typeFilter, setTypeFilter] = useState('all');
    const [searchQuery, setSearchQuery] = useState('');

    // Available resources for the selected type and namespace
    const [availableResources, setAvailableResources] = useState([]);
    const [loadingResources, setLoadingResources] = useState(false);

    // Fetch available resources when type or namespace changes
    useEffect(() => {
        if (!formData.resourceType || !formData.namespace) {
            setAvailableResources([]);
            return;
        }

        const fetchResources = async () => {
            setLoadingResources(true);
            try {
                const resources = await fetchResourcesByType(formData.resourceType, formData.namespace);
                const resourceNames = (resources || [])
                    .map(r => r.metadata?.name)
                    .filter(Boolean)
                    .sort();
                setAvailableResources(resourceNames);
            } catch (err) {
                console.error('[FlowTimeline] Failed to fetch resources:', err);
                setAvailableResources([]);
            } finally {
                setLoadingResources(false);
            }
        };

        fetchResources();
    }, [formData.resourceType, formData.namespace]);

    // Filter namespaces to exclude empty (All Namespaces) option
    const namespaceOptions = useMemo(() =>
        (namespaces || []).filter(ns => ns !== ''),
    [namespaces]);

    const fetchTimeline = useCallback(async () => {
        if (!formData.resourceType || !formData.name) {
            setError('Resource type and name are required');
            return;
        }

        setLoading(true);
        setError(null);

        try {
            const result = await GetFlowTimeline(
                formData.resourceType,
                formData.namespace,
                formData.name,
                durationMinutes,
                includeLogs
            );
            setEntries(result || []);
        } catch (err) {
            setError(err.message || 'Failed to fetch timeline');
            setEntries([]);
        } finally {
            setLoading(false);
        }
    }, [formData, durationMinutes, includeLogs]);

    const toggleEntry = useCallback((index) => {
        setExpandedEntries(prev => {
            const next = new Set(prev);
            if (next.has(index)) {
                next.delete(index);
            } else {
                next.add(index);
            }
            return next;
        });
    }, []);

    const filteredEntries = useMemo(() => {
        return entries.filter(entry => {
            if (severityFilter !== 'all' && entry.severity !== severityFilter) return false;
            if (typeFilter !== 'all' && entry.entryType !== typeFilter) return false;
            if (searchQuery) {
                const query = searchQuery.toLowerCase();
                const matchesMessage = entry.message?.toLowerCase().includes(query);
                const matchesResource = entry.resourceRef?.toLowerCase().includes(query);
                const matchesDetails = entry.details?.toLowerCase().includes(query);
                if (!matchesMessage && !matchesResource && !matchesDetails) return false;
            }
            return true;
        });
    }, [entries, severityFilter, typeFilter, searchQuery]);

    const stats = useMemo(() => {
        const counts = { error: 0, warning: 0, info: 0, event: 0, log: 0 };
        entries.forEach(e => {
            counts[e.severity] = (counts[e.severity] || 0) + 1;
            counts[e.entryType] = (counts[e.entryType] || 0) + 1;
        });
        return counts;
    }, [entries]);

    return (
        <div className="h-full flex flex-col bg-background text-text">
            {/* Header */}
            <div className="flex-shrink-0 border-b border-border p-4">
                <div className="flex items-center justify-between mb-4">
                    <h2 className="text-lg font-semibold flex items-center gap-2">
                        <ClockIcon className="h-5 w-5 text-blue-400" />
                        Flow Timeline
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

                {/* Input Form */}
                <div className="grid grid-cols-1 md:grid-cols-4 gap-3 mb-4">
                    <div>
                        <label className="block text-xs text-gray-400 mb-1">Resource Type</label>
                        <SearchSelect
                            options={RESOURCE_TYPES}
                            value={formData.resourceType}
                            onChange={(val) => setFormData(prev => ({ ...prev, resourceType: val, name: '' }))}
                            placeholder="Select type..."
                            getOptionValue={(rt) => rt.value}
                            getOptionLabel={(rt) => rt.label}
                            preserveOrder={true}
                        />
                    </div>
                    <div>
                        <label className="block text-xs text-gray-400 mb-1">Namespace</label>
                        <SearchSelect
                            options={namespaceOptions}
                            value={formData.namespace}
                            onChange={(val) => setFormData(prev => ({ ...prev, namespace: val, name: '' }))}
                            placeholder="Select namespace..."
                        />
                    </div>
                    <div>
                        <label className="block text-xs text-gray-400 mb-1">
                            Resource Name {loadingResources && <span className="text-gray-500">(loading...)</span>}
                        </label>
                        <SearchSelect
                            options={availableResources}
                            value={formData.name}
                            onChange={(val) => setFormData(prev => ({ ...prev, name: val }))}
                            placeholder={loadingResources ? "Loading..." : "Select resource..."}
                            disabled={loadingResources}
                        />
                    </div>
                    <div className="flex items-end gap-2">
                        <button
                            onClick={fetchTimeline}
                            disabled={loading || !formData.name}
                            className="flex-1 px-4 py-[9px] bg-primary hover:bg-primary/80 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium text-white flex items-center justify-center gap-2 transition-colors"
                        >
                            {loading ? (
                                <ArrowPathIcon className="h-4 w-4 animate-spin" />
                            ) : (
                                <MagnifyingGlassIcon className="h-4 w-4" />
                            )}
                            Analyze
                        </button>
                    </div>
                </div>

                {/* Options */}
                <div className="flex items-center gap-4 text-sm">
                    <div className="flex items-center gap-2">
                        <label className="text-gray-400">Duration:</label>
                        <div className="w-24">
                            <SearchSelect
                                options={DURATION_OPTIONS}
                                value={durationMinutes}
                                onChange={setDurationMinutes}
                                placeholder="Duration"
                                getOptionValue={(d) => d.value}
                                getOptionLabel={(d) => d.label}
                                preserveOrder={true}
                                searchable={false}
                            />
                        </div>
                    </div>
                    <label className="flex items-center gap-2 cursor-pointer">
                        <input
                            type="checkbox"
                            checked={includeLogs}
                            onChange={(e) => setIncludeLogs(e.target.checked)}
                            className="rounded bg-surface border-border"
                        />
                        <span className="text-gray-300">Include error logs</span>
                    </label>
                </div>
            </div>

            {/* Error Display */}
            {error && (
                <div className="flex-shrink-0 p-4 bg-red-900/20 border-b border-red-800/50 text-red-400 text-sm">
                    {error}
                </div>
            )}

            {/* Stats & Filters */}
            {entries.length > 0 && (
                <div className="flex-shrink-0 p-3 border-b border-border bg-surface">
                    <div className="flex items-center justify-between gap-4 flex-wrap">
                        <div className="flex items-center gap-4 text-xs">
                            <span className="text-gray-400">{entries.length} entries</span>
                            <div className="flex items-center gap-2">
                                <span className="flex items-center gap-1 text-red-400">
                                    <ExclamationCircleIcon className="h-3.5 w-3.5" />
                                    {stats.error}
                                </span>
                                <span className="flex items-center gap-1 text-amber-400">
                                    <ExclamationTriangleIcon className="h-3.5 w-3.5" />
                                    {stats.warning}
                                </span>
                                <span className="flex items-center gap-1 text-blue-400">
                                    <InformationCircleIcon className="h-3.5 w-3.5" />
                                    {stats.info}
                                </span>
                            </div>
                        </div>

                        <div className="flex items-center gap-3">
                            <div className="relative">
                                <MagnifyingGlassIcon className="h-4 w-4 absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
                                <input
                                    type="text"
                                    value={searchQuery}
                                    onChange={(e) => setSearchQuery(e.target.value)}
                                    placeholder="Search..."
                                    className="w-48 pl-9 pr-3 py-2 bg-surface border border-border rounded text-sm text-text focus:outline-none focus:border-primary"
                                    autoComplete="off"
                                />
                            </div>
                            <div className="w-32">
                                <SearchSelect
                                    options={SEVERITY_OPTIONS}
                                    value={severityFilter}
                                    onChange={setSeverityFilter}
                                    placeholder="Severity"
                                    getOptionValue={(s) => s.value}
                                    getOptionLabel={(s) => s.label}
                                    preserveOrder={true}
                                    searchable={false}
                                />
                            </div>
                            <div className="w-28">
                                <SearchSelect
                                    options={TYPE_OPTIONS}
                                    value={typeFilter}
                                    onChange={setTypeFilter}
                                    placeholder="Type"
                                    getOptionValue={(t) => t.value}
                                    getOptionLabel={(t) => t.label}
                                    preserveOrder={true}
                                    searchable={false}
                                />
                            </div>
                        </div>
                    </div>
                </div>
            )}

            {/* Timeline Content */}
            <div className="flex-1 min-h-0 overflow-auto p-4">
                {loading ? (
                    <div className="flex items-center justify-center h-32">
                        <ArrowPathIcon className="h-8 w-8 text-gray-500 animate-spin" />
                    </div>
                ) : entries.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                        <ClockIcon className="h-12 w-12 mb-2 opacity-50" />
                        <p>Enter a resource to analyze its timeline</p>
                    </div>
                ) : filteredEntries.length === 0 ? (
                    <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                        <MagnifyingGlassIcon className="h-12 w-12 mb-2 opacity-50" />
                        <p>No entries match your filters</p>
                    </div>
                ) : (
                    <Virtuoso
                        data={filteredEntries}
                        itemContent={(index, entry) => (
                            <TimelineEntry
                                entry={entry}
                                expanded={expandedEntries.has(index)}
                                onToggle={() => toggleEntry(index)}
                            />
                        )}
                    />
                )}
            </div>
        </div>
    );
}
