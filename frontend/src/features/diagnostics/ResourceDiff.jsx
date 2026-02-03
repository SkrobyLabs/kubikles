import React, { useState, useCallback, useMemo, useEffect, useRef } from 'react';
import {
    DiffResources,
    ListNamespacesForContext,
    ListResourceNamesForContext
} from '../../../wailsjs/go/main/App';
import { useK8s } from '../../context/K8sContext';
import Tooltip from '../../components/shared/Tooltip';
import SearchSelect from '../../components/shared/SearchSelect';
import {
    ArrowsRightLeftIcon,
    ArrowPathIcon,
    XMarkIcon,
    DocumentDuplicateIcon,
    CheckCircleIcon,
    ExclamationCircleIcon,
    MinusCircleIcon,
    PlusCircleIcon,
    ArrowsPointingInIcon,
    Squares2X2Icon
} from '@heroicons/react/24/outline';

const RESOURCE_TYPES = [
    { value: 'deployment', label: 'Deployment' },
    { value: 'statefulset', label: 'StatefulSet' },
    { value: 'daemonset', label: 'DaemonSet' },
    { value: 'pod', label: 'Pod' },
    { value: 'service', label: 'Service' },
    { value: 'configmap', label: 'ConfigMap' },
    { value: 'secret', label: 'Secret' },
    { value: 'ingress', label: 'Ingress' },
    { value: 'job', label: 'Job' },
    { value: 'cronjob', label: 'CronJob' },
    { value: 'pvc', label: 'PVC' },
    { value: 'serviceaccount', label: 'ServiceAccount' },
    { value: 'role', label: 'Role' },
    { value: 'rolebinding', label: 'RoleBinding' },
    { value: 'clusterrole', label: 'ClusterRole' },
    { value: 'clusterrolebinding', label: 'ClusterRoleBinding' },
    { value: 'networkpolicy', label: 'NetworkPolicy' },
    { value: 'hpa', label: 'HPA' }
];

// Helper to fetch resource names - uses generic ListResourceNamesForContext for all types.
// This ensures consistent context selection across all resource types.
const fetchResourceNamesByType = async (type, namespace, context = '') => {
    // ListResourceNamesForContext returns [{name, namespace}] for all supported types
    return ListResourceNamesForContext(context, type, namespace);
};

function DiffLine({ line, sourceLineNum, targetLineNum }) {
    // Diff headers - no line number, distinct styling
    if (line.startsWith('---')) {
        return (
            <div className="flex font-mono text-xs bg-red-900/20 text-red-400 border-b border-border">
                <span className="w-8 border-r border-border shrink-0" />
                <span className="w-8 border-r border-border shrink-0" />
                <code className="pl-2 whitespace-pre select-text cursor-text font-semibold">{line}</code>
            </div>
        );
    }
    if (line.startsWith('+++')) {
        return (
            <div className="flex font-mono text-xs bg-green-900/20 text-green-400 border-b border-border">
                <span className="w-8 border-r border-border shrink-0" />
                <span className="w-8 border-r border-border shrink-0" />
                <code className="pl-2 whitespace-pre select-text cursor-text font-semibold">{line}</code>
            </div>
        );
    }
    // Hunk headers (@@ -1,5 +1,5 @@)
    if (line.startsWith('@@')) {
        return (
            <div className="flex font-mono text-xs bg-blue-900/20 text-blue-400">
                <span className="w-8 border-r border-border shrink-0" />
                <span className="w-8 border-r border-border shrink-0" />
                <code className="pl-2 whitespace-pre select-text cursor-text">{line}</code>
            </div>
        );
    }
    // Addition - only target line number
    if (line.startsWith('+')) {
        return (
            <div className="flex font-mono text-xs bg-green-900/30 text-green-300">
                <span className="w-8 text-right pr-1 text-gray-600 select-none border-r border-border shrink-0" />
                <span className="w-8 text-right pr-1 text-green-600 select-none border-r border-border shrink-0">{targetLineNum}</span>
                <code className="pl-2 whitespace-pre select-text cursor-text">{line.substring(1)}</code>
            </div>
        );
    }
    // Deletion - only source line number
    if (line.startsWith('-')) {
        return (
            <div className="flex font-mono text-xs bg-red-900/30 text-red-300">
                <span className="w-8 text-right pr-1 text-red-600 select-none border-r border-border shrink-0">{sourceLineNum}</span>
                <span className="w-8 text-right pr-1 text-gray-600 select-none border-r border-border shrink-0" />
                <code className="pl-2 whitespace-pre select-text cursor-text">{line.substring(1)}</code>
            </div>
        );
    }
    // Context line (starts with space) - both line numbers
    if (line.startsWith(' ')) {
        return (
            <div className="flex font-mono text-xs text-text">
                <span className="w-8 text-right pr-1 text-gray-500 select-none border-r border-border shrink-0">{sourceLineNum}</span>
                <span className="w-8 text-right pr-1 text-gray-500 select-none border-r border-border shrink-0">{targetLineNum}</span>
                <code className="pl-2 whitespace-pre select-text cursor-text">{line.substring(1)}</code>
            </div>
        );
    }
    // Other lines
    return (
        <div className="flex font-mono text-xs text-text">
            <span className="w-8 border-r border-border shrink-0" />
            <span className="w-8 border-r border-border shrink-0" />
            <code className="pl-2 whitespace-pre select-text cursor-text">{line}</code>
        </div>
    );
}

// Parse @@ hunk header to extract starting line numbers
function parseHunkHeader(line) {
    // Format: @@ -startSource,countSource +startTarget,countTarget @@
    const match = line.match(/@@ -(\d+)(?:,\d+)? \+(\d+)(?:,\d+)? @@/);
    if (match) {
        return {
            sourceStart: parseInt(match[1], 10),
            targetStart: parseInt(match[2], 10)
        };
    }
    return null;
}

function UnifiedDiffView({ diffLines }) {
    // Process diff lines and compute line numbers
    const processedLines = useMemo(() => {
        const result = [];
        let sourceLineNum = 0;
        let targetLineNum = 0;

        for (const line of diffLines) {
            // Parse hunk headers to reset line numbers
            if (line.startsWith('@@')) {
                const hunk = parseHunkHeader(line);
                if (hunk) {
                    sourceLineNum = hunk.sourceStart - 1; // -1 because we increment before use
                    targetLineNum = hunk.targetStart - 1;
                }
                result.push({ line, sourceLineNum: null, targetLineNum: null });
                continue;
            }

            // Headers - no line numbers
            if (line.startsWith('---') || line.startsWith('+++') || line.startsWith('\\')) {
                result.push({ line, sourceLineNum: null, targetLineNum: null });
                continue;
            }

            // Deletion - increment source only
            if (line.startsWith('-')) {
                sourceLineNum++;
                result.push({ line, sourceLineNum, targetLineNum: null });
                continue;
            }

            // Addition - increment target only
            if (line.startsWith('+')) {
                targetLineNum++;
                result.push({ line, sourceLineNum: null, targetLineNum });
                continue;
            }

            // Context line - increment both
            if (line.startsWith(' ')) {
                sourceLineNum++;
                targetLineNum++;
                result.push({ line, sourceLineNum, targetLineNum });
                continue;
            }

            // Other lines
            result.push({ line, sourceLineNum: null, targetLineNum: null });
        }

        return result;
    }, [diffLines]);

    if (processedLines.length === 0) {
        return (
            <div className="border border-border rounded overflow-hidden">
                <div className="bg-surface px-3 py-2 text-sm font-medium border-b border-border">
                    Unified Diff
                </div>
                <div className="p-4 text-center text-gray-500">
                    No differences to show
                </div>
            </div>
        );
    }

    return (
        <div className="border border-border rounded overflow-hidden">
            <div className="bg-surface px-3 py-2 text-sm font-medium border-b border-border">
                Unified Diff
            </div>
            <div className="overflow-auto max-h-[60vh]">
                {processedLines.map((item, index) => (
                    <DiffLine
                        key={index}
                        line={item.line}
                        sourceLineNum={item.sourceLineNum}
                        targetLineNum={item.targetLineNum}
                    />
                ))}
            </div>
        </div>
    );
}

function ChangesSummary({ changes }) {
    if (!changes || changes.length === 0) return null;

    const grouped = {
        added: changes.filter(c => c.type === 'added'),
        removed: changes.filter(c => c.type === 'removed'),
        changed: changes.filter(c => c.type === 'changed')
    };

    return (
        <div className="border border-border rounded overflow-hidden">
            <div className="bg-surface px-3 py-2 text-sm font-medium border-b border-border">
                Structured Changes ({changes.length})
            </div>
            <div className="max-h-64 overflow-auto">
                {grouped.added.length > 0 && (
                    <div className="border-b border-border">
                        <div className="px-3 py-1.5 bg-green-900/20 text-green-400 text-xs font-medium flex items-center gap-1.5">
                            <PlusCircleIcon className="h-4 w-4" />
                            Added ({grouped.added.length})
                        </div>
                        {grouped.added.map((c, i) => (
                            <div key={i} className="px-3 py-1 text-xs font-mono border-b border-border/50 last:border-0">
                                <span className="text-gray-400">{c.path}:</span>
                                <span className="text-green-300 ml-2">{c.new}</span>
                            </div>
                        ))}
                    </div>
                )}
                {grouped.removed.length > 0 && (
                    <div className="border-b border-border">
                        <div className="px-3 py-1.5 bg-red-900/20 text-red-400 text-xs font-medium flex items-center gap-1.5">
                            <MinusCircleIcon className="h-4 w-4" />
                            Removed ({grouped.removed.length})
                        </div>
                        {grouped.removed.map((c, i) => (
                            <div key={i} className="px-3 py-1 text-xs font-mono border-b border-border/50 last:border-0">
                                <span className="text-gray-400">{c.path}:</span>
                                <span className="text-red-300 ml-2">{c.old}</span>
                            </div>
                        ))}
                    </div>
                )}
                {grouped.changed.length > 0 && (
                    <div>
                        <div className="px-3 py-1.5 bg-amber-900/20 text-amber-400 text-xs font-medium flex items-center gap-1.5">
                            <ArrowsRightLeftIcon className="h-4 w-4" />
                            Changed ({grouped.changed.length})
                        </div>
                        {grouped.changed.map((c, i) => (
                            <div key={i} className="px-3 py-1 text-xs font-mono border-b border-border/50 last:border-0">
                                <span className="text-gray-400">{c.path}:</span>
                                <span className="text-red-300 ml-2 line-through">{c.old}</span>
                                <span className="text-gray-500 mx-1">-&gt;</span>
                                <span className="text-green-300">{c.new}</span>
                            </div>
                        ))}
                    </div>
                )}
            </div>
        </div>
    );
}

// Parse unified diff into side-by-side aligned lines
// Groups consecutive deletions and additions to show changes on same row
function parseDiffToSideBySide(unifiedDiff) {
    if (!unifiedDiff) return { left: [], right: [] };

    // Split and filter out trailing empty lines from split
    const rawLines = unifiedDiff.split('\n');
    const lines = rawLines.filter((line, idx) => {
        // Keep all lines except trailing empty string from split
        if (idx === rawLines.length - 1 && line === '') return false;
        return true;
    });

    const left = [];  // Source (deletions shown here)
    const right = []; // Target (additions shown here)

    let leftLineNum = 0;
    let rightLineNum = 0;
    let i = 0;

    while (i < lines.length) {
        const line = lines[i];

        // Skip diff headers and "no newline" markers
        if (line.startsWith('---') || line.startsWith('+++') || line.startsWith('@@') || line.startsWith('\\')) {
            i++;
            continue;
        }

        // Collect consecutive deletions
        const deletions = [];
        while (i < lines.length && lines[i].startsWith('-')) {
            deletions.push(lines[i].substring(1));
            i++;
        }

        // Collect consecutive additions
        const additions = [];
        while (i < lines.length && lines[i].startsWith('+')) {
            additions.push(lines[i].substring(1));
            i++;
        }

        // If we found deletions or additions, pair them up
        if (deletions.length > 0 || additions.length > 0) {
            const maxLen = Math.max(deletions.length, additions.length);
            for (let j = 0; j < maxLen; j++) {
                const hasDeletion = j < deletions.length;
                const hasAddition = j < additions.length;

                if (hasDeletion) leftLineNum++;
                if (hasAddition) rightLineNum++;

                left.push({
                    lineNum: hasDeletion ? leftLineNum : null,
                    content: hasDeletion ? deletions[j] : '',
                    type: hasDeletion ? 'removed' : 'empty'
                });
                right.push({
                    lineNum: hasAddition ? rightLineNum : null,
                    content: hasAddition ? additions[j] : '',
                    type: hasAddition ? 'added' : 'empty'
                });
            }
            continue;
        }

        // Context line (starts with space) or other
        if (line.startsWith(' ')) {
            leftLineNum++;
            rightLineNum++;
            left.push({
                lineNum: leftLineNum,
                content: line.substring(1),
                type: 'context'
            });
            right.push({
                lineNum: rightLineNum,
                content: line.substring(1),
                type: 'context'
            });
        } else {
            // Other lines (possibly empty or unknown format) - treat as context
            leftLineNum++;
            rightLineNum++;
            left.push({
                lineNum: leftLineNum,
                content: line,
                type: 'context'
            });
            right.push({
                lineNum: rightLineNum,
                content: line,
                type: 'context'
            });
        }
        i++;
    }

    return { left, right };
}

function SplitDiffLine({ line, side }) {
    const bgClass = line.type === 'removed' ? 'bg-red-900/30' :
                    line.type === 'added' ? 'bg-green-900/30' :
                    line.type === 'empty' ? 'bg-gray-800/30' : '';
    const textClass = line.type === 'removed' ? 'text-red-300' :
                      line.type === 'added' ? 'text-green-300' :
                      line.type === 'empty' ? '' : 'text-text';

    return (
        <div className={`flex font-mono text-xs ${bgClass} ${textClass}`}>
            <span className="w-8 text-right pr-1 text-gray-500 select-none border-r border-border shrink-0">
                {line.lineNum || ''}
            </span>
            <code className="pl-2 whitespace-pre select-text cursor-text">{line.content}</code>
        </div>
    );
}

function SplitDiffView({ unifiedDiff, sourceName, targetName, sourceContext, targetContext }) {
    const { left, right } = useMemo(() => parseDiffToSideBySide(unifiedDiff), [unifiedDiff]);
    const leftRef = useRef(null);
    const rightRef = useRef(null);
    const syncingRef = useRef(false);

    // Show context prefix when contexts differ
    const showContexts = sourceContext && targetContext && sourceContext !== targetContext;
    const sourceLabel = showContexts ? `[${sourceContext}] ${sourceName}` : sourceName;
    const targetLabel = showContexts ? `[${targetContext}] ${targetName}` : targetName;

    // Synchronized scrolling handler
    const handleScroll = useCallback((source) => {
        if (syncingRef.current) return;
        syncingRef.current = true;

        const sourceEl = source === 'left' ? leftRef.current : rightRef.current;
        const targetEl = source === 'left' ? rightRef.current : leftRef.current;

        if (sourceEl && targetEl) {
            targetEl.scrollTop = sourceEl.scrollTop;
        }

        // Use requestAnimationFrame to avoid recursive scroll events
        requestAnimationFrame(() => {
            syncingRef.current = false;
        });
    }, []);

    if (left.length === 0 && right.length === 0) {
        return (
            <div className="p-4 text-center text-gray-500">
                No differences found - resources are identical
            </div>
        );
    }

    return (
        <div className="grid grid-cols-2 gap-0 border border-border rounded">
            {/* Left side - Source */}
            <div className="border-r border-border flex flex-col min-w-0">
                <div className="bg-red-900/20 px-3 py-2 text-sm font-medium border-b border-border text-red-300 shrink-0 truncate">
                    Source: {sourceLabel}
                </div>
                <div
                    ref={leftRef}
                    onScroll={() => handleScroll('left')}
                    className="overflow-auto flex-1"
                    style={{ maxHeight: '60vh' }}
                >
                    {left.map((line, idx) => (
                        <SplitDiffLine key={idx} line={line} side="left" />
                    ))}
                </div>
            </div>
            {/* Right side - Target */}
            <div className="flex flex-col min-w-0">
                <div className="bg-green-900/20 px-3 py-2 text-sm font-medium border-b border-border text-green-300 shrink-0 truncate">
                    Target: {targetLabel}
                </div>
                <div
                    ref={rightRef}
                    onScroll={() => handleScroll('right')}
                    className="overflow-auto flex-1"
                    style={{ maxHeight: '60vh' }}
                >
                    {right.map((line, idx) => (
                        <SplitDiffLine key={idx} line={line} side="right" />
                    ))}
                </div>
            </div>
        </div>
    );
}

function ResourceSelector({ id, label, data, onChange, contexts = [], defaultNamespaces = [] }) {
    const contextOptions = useMemo(() => ['', ...contexts], [contexts]);
    const [availableResources, setAvailableResources] = useState([]);
    const [loadingResources, setLoadingResources] = useState(false);
    const [namespaceOptions, setNamespaceOptions] = useState(defaultNamespaces);
    const [loadingNamespaces, setLoadingNamespaces] = useState(false);

    // Fetch namespaces when context changes
    useEffect(() => {
        const fetchNamespaces = async () => {
            setLoadingNamespaces(true);
            try {
                const namespaces = await ListNamespacesForContext(data.context || '');
                const namespaceNames = (namespaces || [])
                    .map(ns => ns.metadata?.name)
                    .filter(Boolean)
                    .sort();
                setNamespaceOptions(namespaceNames);
            } catch (err) {
                console.error('[ResourceDiff] Failed to fetch namespaces:', err);
                // Fall back to default namespaces on error
                setNamespaceOptions(defaultNamespaces);
            } finally {
                setLoadingNamespaces(false);
            }
        };
        fetchNamespaces();
    }, [data.context, defaultNamespaces]);

    // Fetch function that can be called on mount and on dropdown open
    const fetchResources = useCallback(async () => {
        // Cluster-scoped resources don't need namespace
        const isClusterScoped = data.kind === 'clusterrole' || data.kind === 'clusterrolebinding';
        if (!data.kind || (!isClusterScoped && !data.namespace)) {
            setAvailableResources([]);
            return;
        }

        setLoadingResources(true);
        try {
            // ListResourceNamesForContext returns [{name, namespace}] objects
            const resources = await fetchResourceNamesByType(data.kind, data.namespace, data.context);
            const resourceNames = (resources || [])
                .map(r => r.name)
                .filter(Boolean)
                .sort();
            setAvailableResources(resourceNames);
        } catch (err) {
            console.error('[ResourceDiff] Failed to fetch resources:', err);
            setAvailableResources([]);
        } finally {
            setLoadingResources(false);
        }
    }, [data.kind, data.namespace, data.context]);

    // Fetch on mount and when type/namespace/context changes
    useEffect(() => {
        fetchResources();
    }, [fetchResources]);

    return (
        <div className="space-y-2">
            <h3 className="text-sm font-medium text-text">{label}</h3>
            <div className="grid grid-cols-2 gap-2">
                {contexts.length > 1 && (
                    <div className="col-span-2">
                        <label className="block text-xs text-gray-400 mb-1">Context</label>
                        <SearchSelect
                            key={`${id}-context`}
                            options={contextOptions}
                            value={data.context}
                            onChange={(val) => onChange({ ...data, context: val })}
                            placeholder="(Current)"
                            getOptionLabel={(ctx) => ctx === '' ? '(Current)' : ctx}
                            preserveOrder={true}
                        />
                    </div>
                )}
                <div>
                    <label className="block text-xs text-gray-400 mb-1">Type</label>
                    <SearchSelect
                        key={`${id}-type`}
                        options={RESOURCE_TYPES}
                        value={data.kind}
                        onChange={(val) => onChange({ ...data, kind: val, name: '' })}
                        placeholder="Select type..."
                        getOptionValue={(rt) => rt.value}
                        getOptionLabel={(rt) => rt.label}
                        preserveOrder={true}
                    />
                </div>
                <div>
                    <label className="block text-xs text-gray-400 mb-1">
                        Namespace {loadingNamespaces && <span className="text-gray-500">(loading...)</span>}
                    </label>
                    <SearchSelect
                        key={`${id}-namespace`}
                        options={namespaceOptions}
                        value={data.namespace}
                        onChange={(val) => onChange({ ...data, namespace: val, name: '' })}
                        placeholder={loadingNamespaces ? "Loading..." : "Select namespace..."}
                        disabled={loadingNamespaces}
                    />
                </div>
                <div className="col-span-2">
                    <label className="block text-xs text-gray-400 mb-1">
                        Name {loadingResources && <span className="text-gray-500">(loading...)</span>}
                    </label>
                    <SearchSelect
                        key={`${id}-name`}
                        options={availableResources}
                        value={data.name}
                        onChange={(val) => onChange({ ...data, name: val })}
                        placeholder={loadingResources ? "Loading..." : "Select resource..."}
                        disabled={loadingResources}
                        onOpen={fetchResources}
                    />
                </div>
            </div>
        </div>
    );
}

export default function ResourceDiff({
    initialSource = {},
    initialTarget = {},
    onClose
}) {
    const { currentNamespace, currentContext, contexts, namespaces } = useK8s();

    // Don't use '*' (All Namespaces) as a namespace - fall back to 'default'
    const effectiveNamespace = (currentNamespace && currentNamespace !== '*') ? currentNamespace : 'default';

    // Filter namespaces to exclude empty (All Namespaces) option
    const namespaceOptions = useMemo(() =>
        (namespaces || []).filter(ns => ns !== ''),
    [namespaces]);

    const [source, setSource] = useState({
        context: '',
        kind: 'deployment',
        namespace: effectiveNamespace,
        name: '',
        ...initialSource
    });

    const [target, setTarget] = useState({
        context: '',
        kind: 'deployment',
        namespace: effectiveNamespace,
        name: '',
        ...initialTarget
    });

    const [result, setResult] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);
    const [viewMode, setViewMode] = useState('unified'); // 'unified' | 'split' | 'changes'

    // Track if we should auto-run diff when both source and target are provided
    const autoRunRef = useRef(false);

    // Update state when initial props change (e.g., when navigating from comparison menu)
    useEffect(() => {
        if (initialSource && (initialSource.name || initialSource.kind)) {
            setSource(prev => ({
                ...prev,
                context: initialSource.context || '',
                kind: initialSource.kind || prev.kind,
                namespace: initialSource.namespace || prev.namespace,
                name: initialSource.name || ''
            }));
            // If both source and target have names, auto-run the diff
            if (initialSource.name && initialTarget?.name) {
                autoRunRef.current = true;
            }
        }
    }, [initialSource?.context, initialSource?.kind, initialSource?.namespace, initialSource?.name]);

    useEffect(() => {
        if (initialTarget && (initialTarget.name || initialTarget.kind)) {
            setTarget(prev => ({
                ...prev,
                context: initialTarget.context || '',
                kind: initialTarget.kind || prev.kind,
                namespace: initialTarget.namespace || prev.namespace,
                name: initialTarget.name || ''
            }));
        }
    }, [initialTarget?.context, initialTarget?.kind, initialTarget?.namespace, initialTarget?.name]);

    const performDiff = useCallback(async () => {
        if (!source.name || !target.name) {
            setError('Both source and target resource names are required');
            return;
        }

        setLoading(true);
        setError(null);

        // Use currentContext when context is empty (meaning "current")
        const sourceCtx = source.context || currentContext;
        const targetCtx = target.context || currentContext;

        try {
            const diffResult = await DiffResources(
                sourceCtx,
                source.namespace,
                source.kind,
                source.name,
                targetCtx,
                target.namespace,
                target.kind,
                target.name,
                [] // Use default ignore fields
            );
            setResult(diffResult);
        } catch (err) {
            setError(err.message || 'Failed to compare resources');
            setResult(null);
        } finally {
            setLoading(false);
        }
    }, [source, target, currentContext]);

    // Auto-run diff when both source and target are provided via initial props
    useEffect(() => {
        if (autoRunRef.current && source.name && target.name) {
            autoRunRef.current = false;
            performDiff();
        }
    }, [source.name, target.name, performDiff]);

    const diffLines = useMemo(() => {
        if (!result?.unifiedDiff) return [];
        return result.unifiedDiff.split('\n');
    }, [result]);

    return (
        <div className="h-full flex flex-col bg-background text-text">
            {/* Header */}
            <div className="flex-shrink-0 border-b border-border p-4">
                <div className="flex items-center justify-between mb-4">
                    <h2 className="text-lg font-semibold flex items-center gap-2">
                        <ArrowsRightLeftIcon className="h-5 w-5 text-purple-400" />
                        Resource Diff
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

                {/* Source and Target Selectors */}
                <div className="grid grid-cols-1 md:grid-cols-2 gap-6 mb-4">
                    <ResourceSelector
                        key="source"
                        id="source"
                        label="Source Resource"
                        data={source}
                        onChange={setSource}
                        contexts={contexts}
                        defaultNamespaces={namespaceOptions}
                    />
                    <ResourceSelector
                        key="target"
                        id="target"
                        label="Target Resource"
                        data={target}
                        onChange={setTarget}
                        contexts={contexts}
                        defaultNamespaces={namespaceOptions}
                    />
                </div>

                <div className="flex items-center justify-between">
                    <button
                        onClick={performDiff}
                        disabled={loading || !source.name || !target.name}
                        className="px-4 py-2 bg-primary hover:bg-primary/80 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium text-white flex items-center gap-2 transition-colors"
                    >
                        {loading ? (
                            <ArrowPathIcon className="h-4 w-4 animate-spin" />
                        ) : (
                            <ArrowsRightLeftIcon className="h-4 w-4" />
                        )}
                        Compare
                    </button>

                    {result && (
                        <div className="flex items-center gap-2">
                            <button
                                onClick={() => setViewMode('unified')}
                                className={`px-3 py-1.5 rounded text-sm flex items-center gap-1.5 transition-colors ${
                                    viewMode === 'unified'
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:bg-white/10'
                                }`}
                            >
                                <ArrowsPointingInIcon className="h-4 w-4" />
                                Unified
                            </button>
                            <button
                                onClick={() => setViewMode('split')}
                                className={`px-3 py-1.5 rounded text-sm flex items-center gap-1.5 transition-colors ${
                                    viewMode === 'split'
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:bg-white/10'
                                }`}
                            >
                                <Squares2X2Icon className="h-4 w-4" />
                                Split
                            </button>
                            <button
                                onClick={() => setViewMode('changes')}
                                className={`px-3 py-1.5 rounded text-sm flex items-center gap-1.5 transition-colors ${
                                    viewMode === 'changes'
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:bg-white/10'
                                }`}
                            >
                                <DocumentDuplicateIcon className="h-4 w-4" />
                                Changes
                            </button>
                        </div>
                    )}
                </div>
            </div>

            {/* Error Display */}
            {error && (
                <div className="flex-shrink-0 p-4 bg-red-900/20 border-b border-red-800/50 text-red-400 text-sm flex items-center gap-2">
                    <ExclamationCircleIcon className="h-5 w-5 flex-shrink-0" />
                    {error}
                </div>
            )}

            {/* Result Summary */}
            {result && (
                <div className="flex-shrink-0 p-3 border-b border-border bg-surface">
                    <div className="flex items-center gap-4 text-sm">
                        {!result.sourceExists && (
                            <span className="text-amber-400 flex items-center gap-1">
                                <ExclamationCircleIcon className="h-4 w-4" />
                                Source not found
                            </span>
                        )}
                        {!result.targetExists && (
                            <span className="text-amber-400 flex items-center gap-1">
                                <ExclamationCircleIcon className="h-4 w-4" />
                                Target not found
                            </span>
                        )}
                        {result.sourceExists && result.targetExists && (
                            result.hasChanges ? (
                                <span className="text-amber-400 flex items-center gap-1">
                                    <ArrowsRightLeftIcon className="h-4 w-4" />
                                    {result.changeCount} differences found
                                </span>
                            ) : (
                                <span className="text-green-400 flex items-center gap-1">
                                    <CheckCircleIcon className="h-4 w-4" />
                                    Resources are identical
                                </span>
                            )
                        )}
                    </div>
                </div>
            )}

            {/* Diff Content */}
            <div className="flex-1 min-h-0 overflow-auto p-4">
                {loading ? (
                    <div className="flex items-center justify-center h-32">
                        <ArrowPathIcon className="h-8 w-8 text-gray-500 animate-spin" />
                    </div>
                ) : !result ? (
                    <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                        <ArrowsRightLeftIcon className="h-12 w-12 mb-2 opacity-50" />
                        <p>Select two resources to compare</p>
                    </div>
                ) : viewMode === 'changes' ? (
                    <ChangesSummary changes={result.changes} />
                ) : viewMode === 'split' ? (
                    <SplitDiffView
                        key={`${source.context}-${source.name}-${target.context}-${target.name}`}
                        unifiedDiff={result.unifiedDiff}
                        sourceName={`${source.namespace}/${source.name}`}
                        targetName={`${target.namespace}/${target.name}`}
                        sourceContext={source.context || currentContext}
                        targetContext={target.context || currentContext}
                    />
                ) : (
                    <UnifiedDiffView diffLines={diffLines} />
                )}
            </div>
        </div>
    );
}
