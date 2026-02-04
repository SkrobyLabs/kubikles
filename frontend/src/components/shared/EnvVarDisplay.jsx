import React, { useState, useMemo, useCallback } from 'react';
import {
    ChevronDownIcon,
    ChevronRightIcon,
    ClipboardDocumentIcon,
    CheckIcon,
    DocumentTextIcon,
    LockClosedIcon,
    CubeIcon,
    CpuChipIcon,
    EyeIcon,
    EyeSlashIcon,
    ArrowTopRightOnSquareIcon,
    ExclamationTriangleIcon,
    ArrowPathIcon,
    MagnifyingGlassIcon
} from '@heroicons/react/24/outline';
import { GetConfigMapData, GetSecretData } from '../../../wailsjs/go/main/App';
import { useUI } from '../../context';

// Copy button component
const CopyButton = ({ value, className = '' }) => {
    const [copied, setCopied] = useState(false);

    const handleCopy = async (e) => {
        e.stopPropagation();
        if (!value) return;
        try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <button
            onClick={handleCopy}
            className={`p-1 text-gray-500 hover:text-gray-300 transition-colors ${className}`}
            title={copied ? 'Copied!' : 'Copy to clipboard'}
        >
            {copied ? (
                <CheckIcon className="w-3.5 h-3.5 text-green-400" />
            ) : (
                <ClipboardDocumentIcon className="w-3.5 h-3.5" />
            )}
        </button>
    );
};

// Source badge component
const SourceBadge = ({ type }) => {
    const badges = {
        value: {
            className: 'bg-gray-500/10 text-gray-400 border-gray-500/30',
            icon: null,
            label: 'value'
        },
        configMap: {
            className: 'bg-blue-500/10 text-blue-400 border-blue-500/30',
            icon: DocumentTextIcon,
            label: 'configMap'
        },
        secret: {
            className: 'bg-yellow-500/10 text-yellow-400 border-yellow-500/30',
            icon: LockClosedIcon,
            label: 'secret'
        },
        field: {
            className: 'bg-cyan-500/10 text-cyan-400 border-cyan-500/30',
            icon: CubeIcon,
            label: 'field'
        },
        resource: {
            className: 'bg-purple-500/10 text-purple-400 border-purple-500/30',
            icon: CpuChipIcon,
            label: 'resource'
        },
        inherited: {
            className: 'bg-gray-500/10 text-gray-500 border-gray-500/30',
            icon: null,
            label: 'inherited'
        }
    };

    const badge = badges[type] || badges.value;
    const Icon = badge.icon;

    return (
        <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 text-[10px] rounded border ${badge.className}`}>
            {Icon && <Icon className="w-3 h-3" />}
            {badge.label}
        </span>
    );
};

// Get the field value from pod data
const resolveFieldRef = (pod, fieldPath) => {
    if (!fieldPath) return null;

    const paths = {
        'metadata.name': pod.metadata?.name,
        'metadata.namespace': pod.metadata?.namespace,
        'metadata.uid': pod.metadata?.uid,
        'metadata.labels': pod.metadata?.labels ? JSON.stringify(pod.metadata.labels) : null,
        'metadata.annotations': pod.metadata?.annotations ? JSON.stringify(pod.metadata.annotations) : null,
        'spec.nodeName': pod.spec?.nodeName,
        'spec.serviceAccountName': pod.spec?.serviceAccountName,
        'status.hostIP': pod.status?.hostIP,
        'status.podIP': pod.status?.podIP,
        'status.podIPs': pod.status?.podIPs?.map(ip => ip.ip).join(',')
    };

    // Direct match
    if (paths[fieldPath] !== undefined) {
        return paths[fieldPath];
    }

    // Try to resolve nested paths
    const parts = fieldPath.split('.');
    let current = pod;
    for (const part of parts) {
        // Handle array index notation like labels['app']
        const match = part.match(/^(\w+)\['([^']+)'\]$/);
        if (match) {
            current = current?.[match[1]]?.[match[2]];
        } else {
            current = current?.[part];
        }
        if (current === undefined) return null;
    }

    return typeof current === 'object' ? JSON.stringify(current) : current;
};

// Single environment variable item
const EnvVarItem = ({
    envVar,
    pod,
    namespace,
    isStale,
    resolvedValues,
    onResolve,
    onNavigate
}) => {
    const [revealed, setRevealed] = useState(false);
    const [resolving, setResolving] = useState(false);

    // Determine the type and source of this env var
    const { type, sourceName, sourceKey, displayValue, copyValue } = useMemo(() => {
        // Direct value
        if (envVar.value !== undefined) {
            return {
                type: 'value',
                sourceName: null,
                sourceKey: null,
                displayValue: envVar.value,
                copyValue: envVar.value
            };
        }

        // ConfigMap ref
        if (envVar.valueFrom?.configMapKeyRef) {
            const ref = envVar.valueFrom.configMapKeyRef;
            const resolved = resolvedValues[`configmap:${ref.name}/${ref.key}`];
            return {
                type: 'configMap',
                sourceName: ref.name,
                sourceKey: ref.key,
                displayValue: resolved?.value ?? `$(configmap:${ref.name}/${ref.key})`,
                copyValue: resolved?.value ?? `$(configmap:${ref.name}/${ref.key})`,
                isResolved: resolved?.resolved,
                error: resolved?.error
            };
        }

        // Secret ref
        if (envVar.valueFrom?.secretKeyRef) {
            const ref = envVar.valueFrom.secretKeyRef;
            const resolved = resolvedValues[`secret:${ref.name}/${ref.key}`];
            return {
                type: 'secret',
                sourceName: ref.name,
                sourceKey: ref.key,
                displayValue: resolved?.value ?? `$(secret:${ref.name}/${ref.key})`,
                copyValue: resolved?.value ?? `$(secret:${ref.name}/${ref.key})`,
                isResolved: resolved?.resolved,
                error: resolved?.error
            };
        }

        // Field ref
        if (envVar.valueFrom?.fieldRef) {
            const fieldPath = envVar.valueFrom.fieldRef.fieldPath;
            const resolved = resolveFieldRef(pod, fieldPath);
            return {
                type: 'field',
                sourceName: null,
                sourceKey: fieldPath,
                displayValue: resolved ?? `$(field:${fieldPath})`,
                copyValue: resolved ?? `$(field:${fieldPath})`,
                isResolved: resolved !== null
            };
        }

        // Resource field ref
        if (envVar.valueFrom?.resourceFieldRef) {
            const ref = envVar.valueFrom.resourceFieldRef;
            const path = ref.containerName ? `${ref.containerName}/${ref.resource}` : ref.resource;
            return {
                type: 'resource',
                sourceName: ref.containerName,
                sourceKey: ref.resource,
                displayValue: `$(resource:${path})`,
                copyValue: `$(resource:${path})`
            };
        }

        // No value or valueFrom - inherited from container image
        return {
            type: 'inherited',
            sourceName: null,
            sourceKey: null,
            displayValue: null,
            copyValue: ''
        };
    }, [envVar, pod, resolvedValues]);

    const handleResolve = async () => {
        if (isStale || resolving) return;
        setResolving(true);
        try {
            if (type === 'configMap') {
                await onResolve('configMap', sourceName, sourceKey);
            } else if (type === 'secret') {
                await onResolve('secret', sourceName, sourceKey);
            }
        } finally {
            setResolving(false);
        }
    };

    const handleNavigate = () => {
        if (!sourceName) return;
        if (type === 'configMap') {
            onNavigate('configmaps', `name:"${sourceName}" namespace:"${namespace}"`);
        } else if (type === 'secret') {
            onNavigate('secrets', `name:"${sourceName}" namespace:"${namespace}"`);
        }
    };

    const isSecret = type === 'secret';
    const hasError = !!envVar.error;
    const needsResolve = (type === 'configMap' || type === 'secret') && !envVar.isResolved && !hasError;
    const showMasked = isSecret && envVar.isResolved && !revealed;

    return (
        <div className="flex items-start gap-2 py-1.5 border-b border-border/30 last:border-b-0">
            {/* Name */}
            <code className="font-mono text-xs text-gray-200 shrink-0 max-w-[26rem] truncate" title={envVar.name}>
                {envVar.name}
            </code>

            {/* Source badge */}
            <SourceBadge type={type} />

            {/* Value display */}
            <div className="flex-1 min-w-0">
                {hasError ? (
                    <div className="flex items-center gap-1 text-red-400 text-xs">
                        <ExclamationTriangleIcon className="w-3.5 h-3.5" />
                        <span>{envVar.error}</span>
                    </div>
                ) : showMasked ? (
                    <span className="text-xs text-yellow-400 font-mono bg-yellow-500/10 px-1 rounded">
                        ••••••••
                    </span>
                ) : type === 'inherited' ? (
                    <span className="text-xs text-gray-500 italic">(from image)</span>
                ) : (
                    <code className={`text-xs break-all font-mono ${
                        (type === 'configMap' || type === 'secret') && !envVar.isResolved
                            ? 'text-gray-500 italic'
                            : isSecret && revealed
                                ? 'text-yellow-300 bg-yellow-500/10 px-1 rounded'
                                : 'text-gray-300'
                    }`}>
                        {displayValue || <span className="text-gray-500 italic">(empty)</span>}
                    </code>
                )}
            </div>

            {/* Actions */}
            <div className="flex items-center gap-0.5 shrink-0">
                {/* Resolve button for configMap/secret refs */}
                {needsResolve && (
                    <button
                        onClick={handleResolve}
                        disabled={isStale || resolving}
                        className={`p-1 transition-colors rounded ${
                            isStale
                                ? 'text-gray-600 cursor-not-allowed'
                                : 'text-gray-500 hover:text-gray-300 hover:bg-white/5'
                        }`}
                        title={isStale ? 'Cannot resolve in stale context' : 'Resolve value'}
                    >
                        <ArrowPathIcon className={`w-3.5 h-3.5 ${resolving ? 'animate-spin' : ''}`} />
                    </button>
                )}

                {/* Navigate to resource */}
                {(type === 'configMap' || type === 'secret') && sourceName && (
                    <button
                        onClick={handleNavigate}
                        className="p-1 text-gray-500 hover:text-primary transition-colors rounded hover:bg-white/5"
                        title={`Go to ${type === 'configMap' ? 'ConfigMap' : 'Secret'}: ${sourceName}`}
                    >
                        <ArrowTopRightOnSquareIcon className="w-3.5 h-3.5" />
                    </button>
                )}

                {/* Reveal/hide for secrets */}
                {isSecret && envVar.isResolved && !hasError && (
                    <button
                        onClick={() => setRevealed(!revealed)}
                        className="p-1 text-gray-500 hover:text-yellow-400 transition-colors rounded hover:bg-white/5"
                        title={revealed ? 'Hide secret' : 'Reveal secret'}
                    >
                        {revealed ? (
                            <EyeSlashIcon className="w-3.5 h-3.5" />
                        ) : (
                            <EyeIcon className="w-3.5 h-3.5" />
                        )}
                    </button>
                )}

                {/* Copy button */}
                {(envVar.isResolved || type === 'value' || type === 'field') && !hasError && (
                    <CopyButton value={copyValue} />
                )}
            </div>
        </div>
    );
};

// EnvFrom item (bulk import from ConfigMap/Secret)
const EnvFromItem = ({
    envFrom,
    namespace,
    isStale,
    resolvedEnvFrom,
    onResolveEnvFrom,
    onNavigate
}) => {
    const [expanded, setExpanded] = useState(false);
    const [resolving, setResolving] = useState(false);
    const [revealed, setRevealed] = useState(false);

    const { type, sourceName, prefix } = useMemo(() => {
        if (envFrom.configMapRef) {
            return {
                type: 'configMap',
                sourceName: envFrom.configMapRef.name,
                prefix: envFrom.prefix || ''
            };
        }
        if (envFrom.secretRef) {
            return {
                type: 'secret',
                sourceName: envFrom.secretRef.name,
                prefix: envFrom.prefix || ''
            };
        }
        return { type: 'unknown', sourceName: null, prefix: '' };
    }, [envFrom]);

    const resolvedData = resolvedEnvFrom[`${type}:${sourceName}`];
    const isResolved = !!resolvedData?.resolved;
    const hasError = !!resolvedData?.error;
    const entries = resolvedData?.entries || [];

    const handleResolve = async () => {
        if (isStale || resolving) return;
        setResolving(true);
        try {
            await onResolveEnvFrom(type, sourceName);
        } finally {
            setResolving(false);
        }
    };

    const handleNavigate = () => {
        if (!sourceName) return;
        if (type === 'configMap') {
            onNavigate('configmaps', `name:"${sourceName}" namespace:"${namespace}"`);
        } else if (type === 'secret') {
            onNavigate('secrets', `name:"${sourceName}" namespace:"${namespace}"`);
        }
    };

    const isSecret = type === 'secret';

    return (
        <div className="border border-border/50 rounded-lg overflow-hidden">
            {/* Header */}
            <div
                className="flex items-center gap-2 px-3 py-2 bg-surface-light cursor-pointer hover:bg-white/5"
                onClick={() => isResolved && setExpanded(!expanded)}
            >
                {isResolved && entries.length > 0 && (
                    expanded ? (
                        <ChevronDownIcon className="w-4 h-4 text-gray-500" />
                    ) : (
                        <ChevronRightIcon className="w-4 h-4 text-gray-500" />
                    )
                )}

                <SourceBadge type={type} />

                <span className="text-sm text-gray-300 font-medium">{sourceName}</span>

                {prefix && (
                    <span className="text-xs text-gray-500">prefix: {prefix}</span>
                )}

                {isResolved && (
                    <span className="text-xs text-gray-500">({entries.length} keys)</span>
                )}

                <div className="flex-1" />

                {hasError && (
                    <span className="text-xs text-red-400 flex items-center gap-1">
                        <ExclamationTriangleIcon className="w-3.5 h-3.5" />
                        {resolvedData.error}
                    </span>
                )}

                <div className="flex items-center gap-0.5">
                    {!isResolved && !hasError && (
                        <button
                            onClick={(e) => { e.stopPropagation(); handleResolve(); }}
                            disabled={isStale || resolving}
                            className={`p-1 transition-colors rounded ${
                                isStale
                                    ? 'text-gray-600 cursor-not-allowed'
                                    : 'text-gray-500 hover:text-gray-300 hover:bg-white/5'
                            }`}
                            title={isStale ? 'Cannot resolve in stale context' : 'Resolve all keys'}
                        >
                            <ArrowPathIcon className={`w-3.5 h-3.5 ${resolving ? 'animate-spin' : ''}`} />
                        </button>
                    )}

                    {isSecret && isResolved && entries.length > 0 && (
                        <button
                            onClick={(e) => { e.stopPropagation(); setRevealed(!revealed); }}
                            className="p-1 text-gray-500 hover:text-yellow-400 transition-colors rounded hover:bg-white/5"
                            title={revealed ? 'Hide all secrets' : 'Reveal all secrets'}
                        >
                            {revealed ? (
                                <EyeSlashIcon className="w-3.5 h-3.5" />
                            ) : (
                                <EyeIcon className="w-3.5 h-3.5" />
                            )}
                        </button>
                    )}

                    <button
                        onClick={(e) => { e.stopPropagation(); handleNavigate(); }}
                        className="p-1 text-gray-500 hover:text-primary transition-colors rounded hover:bg-white/5"
                        title={`Go to ${type === 'configMap' ? 'ConfigMap' : 'Secret'}: ${sourceName}`}
                    >
                        <ArrowTopRightOnSquareIcon className="w-3.5 h-3.5" />
                    </button>
                </div>
            </div>

            {/* Expanded entries */}
            {expanded && entries.length > 0 && (
                <div className="border-t border-border/50 px-3 py-2 bg-surface/50">
                    {entries.map(({ key, value }) => (
                        <div key={key} className="flex items-center gap-2 py-1 border-b border-border/20 last:border-b-0">
                            <code className="font-mono text-xs text-gray-200 shrink-0 max-w-[26rem] truncate" title={prefix + key}>
                                {prefix}{key}
                            </code>
                            <code className={`flex-1 text-xs font-mono break-all ${
                                isSecret && !revealed
                                    ? 'text-yellow-400 bg-yellow-500/10 px-1 rounded'
                                    : isSecret && revealed
                                        ? 'text-yellow-300 bg-yellow-500/10 px-1 rounded'
                                        : 'text-gray-300'
                            }`}>
                                {isSecret && !revealed ? '••••••••' : value || <span className="text-gray-500 italic">(empty)</span>}
                            </code>
                            {(!isSecret || revealed) && <CopyButton value={value} />}
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
};

// Main component
export default function EnvVarSection({
    env = [],
    envFrom = [],
    pod,
    namespace,
    isStale = false
}) {
    const { navigateWithSearch } = useUI();
    const [expanded, setExpanded] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const [resolvedValues, setResolvedValues] = useState({});
    const [resolvedEnvFrom, setResolvedEnvFrom] = useState({});
    const [resolvingAll, setResolvingAll] = useState(false);

    const totalCount = env.length + envFrom.length;
    const COLLAPSE_THRESHOLD = 50;

    // Filter env vars by search
    const filteredEnv = useMemo(() => {
        if (!searchTerm) return env;
        const term = searchTerm.toLowerCase();
        return env.filter(e =>
            e.name.toLowerCase().includes(term) ||
            e.value?.toLowerCase().includes(term)
        );
    }, [env, searchTerm]);

    // Resolve a single ConfigMap/Secret key
    const handleResolve = useCallback(async (type, name, key) => {
        const cacheKey = `${type === 'configMap' ? 'configmap' : 'secret'}:${name}/${key}`;

        try {
            let data;
            if (type === 'configMap') {
                data = await GetConfigMapData(namespace, name);
            } else {
                data = await GetSecretData(namespace, name);
            }

            if (data && key in data) {
                setResolvedValues(prev => ({
                    ...prev,
                    [cacheKey]: { resolved: true, value: data[key] }
                }));
            } else {
                setResolvedValues(prev => ({
                    ...prev,
                    [cacheKey]: { resolved: true, value: null, error: 'Key not found' }
                }));
            }
        } catch (err) {
            setResolvedValues(prev => ({
                ...prev,
                [cacheKey]: { resolved: true, error: err.message || 'Failed to resolve' }
            }));
        }
    }, [namespace]);

    // Resolve envFrom (entire ConfigMap/Secret)
    const handleResolveEnvFrom = useCallback(async (type, name) => {
        const cacheKey = `${type}:${name}`;

        try {
            let data;
            if (type === 'configMap') {
                data = await GetConfigMapData(namespace, name);
            } else {
                data = await GetSecretData(namespace, name);
            }

            if (data) {
                const entries = Object.entries(data).map(([key, value]) => ({ key, value }));
                setResolvedEnvFrom(prev => ({
                    ...prev,
                    [cacheKey]: { resolved: true, entries }
                }));
            } else {
                setResolvedEnvFrom(prev => ({
                    ...prev,
                    [cacheKey]: { resolved: true, entries: [], error: 'Resource not found' }
                }));
            }
        } catch (err) {
            setResolvedEnvFrom(prev => ({
                ...prev,
                [cacheKey]: { resolved: true, error: err.message || 'Failed to resolve' }
            }));
        }
    }, [namespace]);

    // Resolve all ConfigMap/Secret references
    const handleResolveAll = async () => {
        if (isStale || resolvingAll) return;
        setResolvingAll(true);

        try {
            // Collect all unique ConfigMaps and Secrets to fetch
            const toFetch = new Map();

            for (const e of env) {
                if (e.valueFrom?.configMapKeyRef) {
                    const ref = e.valueFrom.configMapKeyRef;
                    const key = `configMap:${ref.name}`;
                    if (!toFetch.has(key)) {
                        toFetch.set(key, { type: 'configMap', name: ref.name, keys: new Set() });
                    }
                    toFetch.get(key).keys.add(ref.key);
                }
                if (e.valueFrom?.secretKeyRef) {
                    const ref = e.valueFrom.secretKeyRef;
                    const key = `secret:${ref.name}`;
                    if (!toFetch.has(key)) {
                        toFetch.set(key, { type: 'secret', name: ref.name, keys: new Set() });
                    }
                    toFetch.get(key).keys.add(ref.key);
                }
            }

            for (const ef of envFrom) {
                if (ef.configMapRef) {
                    const key = `configMap:${ef.configMapRef.name}`;
                    if (!toFetch.has(key)) {
                        toFetch.set(key, { type: 'configMap', name: ef.configMapRef.name, keys: new Set() });
                    }
                }
                if (ef.secretRef) {
                    const key = `secret:${ef.secretRef.name}`;
                    if (!toFetch.has(key)) {
                        toFetch.set(key, { type: 'secret', name: ef.secretRef.name, keys: new Set() });
                    }
                }
            }

            // Fetch all in parallel
            const promises = Array.from(toFetch.values()).map(async ({ type, name, keys }) => {
                try {
                    let data;
                    if (type === 'configMap') {
                        data = await GetConfigMapData(namespace, name);
                    } else {
                        data = await GetSecretData(namespace, name);
                    }

                    // Update resolved values for individual keys
                    const newResolved = {};
                    for (const key of keys) {
                        const cacheKey = `${type === 'configMap' ? 'configmap' : 'secret'}:${name}/${key}`;
                        if (data && key in data) {
                            newResolved[cacheKey] = { resolved: true, value: data[key] };
                        } else {
                            newResolved[cacheKey] = { resolved: true, value: null, error: 'Key not found' };
                        }
                    }
                    setResolvedValues(prev => ({ ...prev, ...newResolved }));

                    // Update envFrom resolution
                    if (data) {
                        const entries = Object.entries(data).map(([key, value]) => ({ key, value }));
                        setResolvedEnvFrom(prev => ({
                            ...prev,
                            [`${type}:${name}`]: { resolved: true, entries }
                        }));
                    }
                } catch (err) {
                    // Mark all keys from this resource as error
                    const newResolved = {};
                    for (const key of keys) {
                        const cacheKey = `${type === 'configMap' ? 'configmap' : 'secret'}:${name}/${key}`;
                        newResolved[cacheKey] = { resolved: true, error: err.message || 'Failed to resolve' };
                    }
                    setResolvedValues(prev => ({ ...prev, ...newResolved }));
                    setResolvedEnvFrom(prev => ({
                        ...prev,
                        [`${type}:${name}`]: { resolved: true, error: err.message || 'Failed to resolve' }
                    }));
                }
            });

            await Promise.all(promises);
        } finally {
            setResolvingAll(false);
        }
    };

    // Check if there are any unresolved refs
    const hasUnresolvedRefs = useMemo(() => {
        for (const e of env) {
            if (e.valueFrom?.configMapKeyRef) {
                const ref = e.valueFrom.configMapKeyRef;
                if (!resolvedValues[`configmap:${ref.name}/${ref.key}`]?.resolved) return true;
            }
            if (e.valueFrom?.secretKeyRef) {
                const ref = e.valueFrom.secretKeyRef;
                if (!resolvedValues[`secret:${ref.name}/${ref.key}`]?.resolved) return true;
            }
        }
        for (const ef of envFrom) {
            if (ef.configMapRef && !resolvedEnvFrom[`configMap:${ef.configMapRef.name}`]?.resolved) return true;
            if (ef.secretRef && !resolvedEnvFrom[`secret:${ef.secretRef.name}`]?.resolved) return true;
        }
        return false;
    }, [env, envFrom, resolvedValues, resolvedEnvFrom]);

    // Add resolved status to env vars for rendering
    const enrichedEnv = useMemo(() => {
        return filteredEnv.map(e => {
            if (e.valueFrom?.configMapKeyRef) {
                const ref = e.valueFrom.configMapKeyRef;
                const resolved = resolvedValues[`configmap:${ref.name}/${ref.key}`];
                return { ...e, isResolved: resolved?.resolved, error: resolved?.error };
            }
            if (e.valueFrom?.secretKeyRef) {
                const ref = e.valueFrom.secretKeyRef;
                const resolved = resolvedValues[`secret:${ref.name}/${ref.key}`];
                return { ...e, isResolved: resolved?.resolved, error: resolved?.error };
            }
            return e;
        });
    }, [filteredEnv, resolvedValues]);

    if (totalCount === 0) {
        return null;
    }

    const showExpanded = expanded || totalCount <= COLLAPSE_THRESHOLD;
    const displayEnv = showExpanded ? enrichedEnv : enrichedEnv.slice(0, COLLAPSE_THRESHOLD);

    return (
        <div>
            {/* Header */}
            <div className="flex items-center gap-2 mb-3">
                <span className="text-xs text-gray-500">
                    {env.length} variable{env.length !== 1 ? 's' : ''}
                    {envFrom.length > 0 && `, ${envFrom.length} envFrom`}
                </span>

                {/* Search */}
                {env.length > 3 && (
                    <div className="relative">
                        <MagnifyingGlassIcon className="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-gray-500" />
                        <input
                            type="text"
                            value={searchTerm}
                            onChange={(e) => setSearchTerm(e.target.value)}
                            placeholder="Filter..."
                            className="pl-7 pr-2 py-1 text-xs bg-surface border border-border rounded w-32 focus:outline-none focus:border-primary"
                            autoComplete="off"
                            autoCorrect="off"
                            autoCapitalize="off"
                            spellCheck="false"
                        />
                    </div>
                )}

                <div className="flex-1" />

                {/* Resolve All button */}
                {hasUnresolvedRefs && (
                    <button
                        onClick={handleResolveAll}
                        disabled={isStale || resolvingAll}
                        className={`text-xs px-2 py-1 rounded border transition-colors ${
                            isStale
                                ? 'text-gray-600 border-gray-700 cursor-not-allowed'
                                : 'text-gray-400 border-border hover:text-gray-200 hover:border-gray-500'
                        }`}
                    >
                        {resolvingAll ? (
                            <span className="flex items-center gap-1">
                                <ArrowPathIcon className="w-3 h-3 animate-spin" />
                                Resolving...
                            </span>
                        ) : (
                            'Resolve All'
                        )}
                    </button>
                )}
            </div>

            {/* Env vars list */}
            <div className="space-y-0">
                {displayEnv.map((e, idx) => (
                    <EnvVarItem
                        key={e.name || idx}
                        envVar={e}
                        pod={pod}
                        namespace={namespace}
                        isStale={isStale}
                        resolvedValues={resolvedValues}
                        onResolve={handleResolve}
                        onNavigate={navigateWithSearch}
                    />
                ))}
            </div>

            {/* Expand/collapse button */}
            {totalCount > COLLAPSE_THRESHOLD && !showExpanded && (
                <button
                    onClick={() => setExpanded(true)}
                    className="w-full mt-2 py-1 text-xs text-gray-500 hover:text-gray-300 transition-colors flex items-center justify-center gap-1"
                >
                    <ChevronDownIcon className="w-4 h-4" />
                    Show {enrichedEnv.length - COLLAPSE_THRESHOLD} more
                </button>
            )}

            {showExpanded && totalCount > COLLAPSE_THRESHOLD && (
                <button
                    onClick={() => setExpanded(false)}
                    className="w-full mt-2 py-1 text-xs text-gray-500 hover:text-gray-300 transition-colors flex items-center justify-center gap-1"
                >
                    <ChevronDownIcon className="w-4 h-4 rotate-180" />
                    Show less
                </button>
            )}

            {/* EnvFrom section */}
            {envFrom.length > 0 && (
                <div className="mt-3 space-y-2">
                    <div className="text-xs text-gray-500 mb-2">envFrom:</div>
                    {envFrom.map((ef, idx) => (
                        <EnvFromItem
                            key={ef.configMapRef?.name || ef.secretRef?.name || idx}
                            envFrom={ef}
                            namespace={namespace}
                            isStale={isStale}
                            resolvedEnvFrom={resolvedEnvFrom}
                            onResolveEnvFrom={handleResolveEnvFrom}
                            onNavigate={navigateWithSearch}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}
