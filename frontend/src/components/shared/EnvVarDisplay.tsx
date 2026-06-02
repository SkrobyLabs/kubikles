import React, { useState, useMemo, useCallback } from 'react';
import {
    ChevronDownIcon,
    ChevronRightIcon,
    CheckIcon,
    DocumentTextIcon,
    LockClosedIcon,
    CubeIcon,
    CpuChipIcon,
    ExclamationTriangleIcon,
    ArrowPathIcon,
    MagnifyingGlassIcon
} from '@heroicons/react/24/outline';
import { GetConfigMapData, GetSecretData } from 'wailsjs/go/main/App';

interface ResolveResult {
    value?: string | null;
    entries?: Array<{ key: string; value: string }>;
    error?: string;
    copied?: boolean;
}

const cachePrefix = (type: string) => type === 'configMap' ? 'configmap' : 'secret';

const hasConcreteValue = (value: any): boolean => value !== undefined && value !== null;

const entryTextValue = (entry: any) => {
    if (!entry || entry.isBinary || entry.encoding === 'base64' || entry.source === 'binaryData') {
        return undefined;
    }
    return entry.value;
};

const entryTextMap = (entries: any) => {
    if (!Array.isArray(entries)) {
        return entries || {};
    }

    return entries.reduce((acc: Record<string, string>, entry: any) => {
        const value = entryTextValue(entry);
        if (entry.key && value !== undefined) {
            acc[entry.key] = value;
        }
        return acc;
    }, {});
};

const copyToClipboard = async (value: any): Promise<boolean> => {
    if (!hasConcreteValue(value)) return false;
    try {
        await navigator.clipboard.writeText(String(value));
        return true;
    } catch (err: any) {
        console.error('Failed to copy:', err);
        return false;
    }
};

const displayValueText = (value: any): React.ReactNode => {
    if (value === '') return <span className="text-gray-500 italic">(empty)</span>;
    return value;
};

const ClickableEnvValue = ({
    children,
    title,
    disabled = false,
    resolving = false,
    error,
    className = '',
    onCopy
}: {
    children: React.ReactNode;
    title?: string;
    disabled?: boolean;
    resolving?: boolean;
    error?: string;
    className?: string;
    onCopy?: () => Promise<ResolveResult | void>;
}) => {
    const [copied, setCopied] = useState(false);
    const [localError, setLocalError] = useState('');
    const isDisabled = disabled || resolving || !onCopy;
    const errorText = error || localError;

    const handleClick = async (e: any) => {
        e.stopPropagation();
        if (isDisabled) return;

        setLocalError('');
        const result = await onCopy?.();
        if (result?.error) {
            setLocalError(result.error);
            return;
        }

        if (result?.copied !== false) {
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        }
    };

    if (errorText) {
        return (
            <div className="flex items-center gap-1 text-red-400 text-xs">
                <ExclamationTriangleIcon className="w-3.5 h-3.5 shrink-0" />
                <span>{errorText}</span>
            </div>
        );
    }

    if (!onCopy) {
        return (
            <code className={`text-xs break-all font-mono ${className}`}>
                {children}
            </code>
        );
    }

    return (
        <button
            type="button"
            onClick={handleClick}
            disabled={isDisabled}
            className={`inline-flex min-w-0 items-center gap-1 px-1.5 py-0.5 text-xs rounded border font-mono text-left transition-colors ${
                copied
                    ? 'bg-green-500/20 text-green-400 border-green-500/30'
                    : isDisabled
                        ? 'bg-gray-500/10 text-gray-500 border-gray-500/20 cursor-not-allowed'
                        : `bg-gray-500/10 hover:bg-gray-500/20 text-gray-300 border-gray-500/30 cursor-pointer ${className}`
            }`}
            title={copied ? 'Copied' : title}
        >
            {copied ? (
                <>
                    <CheckIcon className="w-3 h-3 shrink-0" />
                    <span>Copied</span>
                </>
            ) : resolving ? (
                <>
                    <ArrowPathIcon className="w-3 h-3 shrink-0 animate-spin" />
                    <span>Resolving...</span>
                </>
            ) : (
                <span className="break-all">{children}</span>
            )}
        </button>
    );
};

const SourceBadge = ({ type }: { type: string }) => {
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

    const badge = (badges as Record<string, any>)[type] || badges.value;
    const Icon = badge.icon;

    return (
        <span className={`inline-flex items-center gap-1 px-1.5 py-0.5 text-[10px] rounded border ${badge.className}`}>
            {Icon && <Icon className="w-3 h-3" />}
            {badge.label}
        </span>
    );
};

const resolveFieldRef = (pod: any, fieldPath: string) => {
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
        'status.podIPs': pod.status?.podIPs?.map((ip: any) => ip.ip).join(',')
    };

    if ((paths as Record<string, any>)[fieldPath] !== undefined) {
        return (paths as Record<string, any>)[fieldPath];
    }

    const parts = fieldPath.split('.');
    let current = pod;
    for (const part of parts) {
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

const EnvVarItem = ({
    envVar,
    pod,
    isStale,
    resolvedValues,
    onResolve
}: {
    envVar: any;
    pod: any;
    isStale: boolean;
    resolvedValues: Record<string, any>;
    onResolve: (type: string, name: string | null, key: string | null) => Promise<ResolveResult>;
}) => {
    const [resolving, setResolving] = useState(false);

    const info = useMemo(() => {
        if (envVar.value !== undefined) {
            return {
                type: 'value',
                sourceName: null,
                sourceKey: null,
                displayValue: envVar.value,
                copyValue: envVar.value,
                canCopy: true,
                placeholder: ''
            };
        }

        if (envVar.valueFrom?.configMapKeyRef) {
            const ref = envVar.valueFrom.configMapKeyRef;
            const placeholder = `$(configmap:${ref.name}/${ref.key})`;
            const resolved = resolvedValues[`configmap:${ref.name}/${ref.key}`];
            return {
                type: 'configMap',
                sourceName: ref.name,
                sourceKey: ref.key,
                displayValue: hasConcreteValue(resolved?.value) ? resolved.value : placeholder,
                copyValue: resolved?.value,
                canCopy: true,
                placeholder,
                error: resolved?.error,
                isResolved: resolved?.resolved
            };
        }

        if (envVar.valueFrom?.secretKeyRef) {
            const ref = envVar.valueFrom.secretKeyRef;
            const placeholder = `$(secret:${ref.name}/${ref.key})`;
            const resolved = resolvedValues[`secret:${ref.name}/${ref.key}`];
            return {
                type: 'secret',
                sourceName: ref.name,
                sourceKey: ref.key,
                displayValue: placeholder,
                copyValue: resolved?.value,
                canCopy: true,
                placeholder,
                error: resolved?.error,
                isResolved: resolved?.resolved
            };
        }

        if (envVar.valueFrom?.fieldRef) {
            const fieldPath = envVar.valueFrom.fieldRef.fieldPath;
            const resolved = resolveFieldRef(pod, fieldPath);
            return {
                type: 'field',
                sourceName: null,
                sourceKey: fieldPath,
                displayValue: hasConcreteValue(resolved) ? resolved : `$(field:${fieldPath})`,
                copyValue: resolved,
                canCopy: hasConcreteValue(resolved),
                placeholder: `$(field:${fieldPath})`,
                isResolved: hasConcreteValue(resolved)
            };
        }

        if (envVar.valueFrom?.resourceFieldRef) {
            const ref = envVar.valueFrom.resourceFieldRef;
            const path = ref.containerName ? `${ref.containerName}/${ref.resource}` : ref.resource;
            return {
                type: 'resource',
                sourceName: ref.containerName,
                sourceKey: ref.resource,
                displayValue: `$(resource:${path})`,
                copyValue: null,
                canCopy: false,
                placeholder: `$(resource:${path})`
            };
        }

        return {
            type: 'inherited',
            sourceName: null,
            sourceKey: null,
            displayValue: '(from image)',
            copyValue: null,
            canCopy: false,
            placeholder: ''
        };
    }, [envVar, pod, resolvedValues]);

    const handleCopy = async (): Promise<ResolveResult> => {
        if (info.type === 'configMap' || info.type === 'secret') {
            if (isStale && !hasConcreteValue(info.copyValue)) {
                return { error: 'Cannot resolve in stale context' };
            }

            if (hasConcreteValue(info.copyValue)) {
                const copied = await copyToClipboard(info.copyValue);
                return copied ? {} : { error: 'Failed to copy' };
            }

            setResolving(true);
            try {
                const result = await onResolve(info.type, info.sourceName, info.sourceKey);
                if (result.error) return result;
                if (!hasConcreteValue(result.value)) return { error: 'Key not found' };
                const copied = await copyToClipboard(result.value);
                return copied ? {} : { error: 'Failed to copy' };
            } finally {
                setResolving(false);
            }
        }

        if (!hasConcreteValue(info.copyValue)) {
            return { error: 'No value to copy' };
        }

        const copied = await copyToClipboard(info.copyValue);
        return copied ? {} : { error: 'Failed to copy' };
    };

    const valueClass = info.type === 'secret'
        ? 'bg-yellow-500/10 text-yellow-300 border-yellow-500/30'
        : (info.type === 'configMap' && !info.isResolved)
            ? 'text-gray-500 italic'
            : 'text-gray-300';

    return (
        <div className="flex items-start gap-2 py-1.5 border-b border-border/30 last:border-b-0">
            <code className="font-mono text-xs text-gray-200 shrink-0 max-w-[26rem] truncate" title={envVar.name}>
                {envVar.name}
            </code>

            <SourceBadge type={info.type} />

            <div className="flex-1 min-w-0">
                {info.type === 'inherited' ? (
                    <span className="text-xs text-gray-500 italic">(from image)</span>
                ) : (
                    <ClickableEnvValue
                        title={info.canCopy ? 'Click to copy value' : undefined}
                        disabled={(info.type === 'configMap' || info.type === 'secret') && isStale && !hasConcreteValue(info.copyValue)}
                        resolving={resolving}
                        error={info.error}
                        className={valueClass}
                        onCopy={info.canCopy ? handleCopy : undefined}
                    >
                        {displayValueText(info.displayValue)}
                    </ClickableEnvValue>
                )}
            </div>
        </div>
    );
};

const EnvFromItem = ({
    envFrom,
    isStale,
    resolvedEnvFrom,
    onResolveEnvFrom
}: {
    envFrom: any;
    isStale: boolean;
    resolvedEnvFrom: Record<string, any>;
    onResolveEnvFrom: (type: string, name: string | null) => Promise<ResolveResult>;
}) => {
    const [expanded, setExpanded] = useState(false);
    const [resolving, setResolving] = useState(false);

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
    const isSecret = type === 'secret';

    const resolveAndExpand = async (): Promise<ResolveResult> => {
        if (isResolved) {
            setExpanded((prev) => entries.length > 0 ? !prev : prev);
            return { copied: false };
        }
        if (isStale) return { error: 'Cannot resolve in stale context' };

        setResolving(true);
        try {
            const result = await onResolveEnvFrom(type, sourceName);
            if (result.error) return result;
            if ((result.entries || []).length > 0) {
                setExpanded(true);
            }
            return { copied: false };
        } finally {
            setResolving(false);
        }
    };

    const copyEntry = (value: string) => async (): Promise<ResolveResult> => {
        const copied = await copyToClipboard(value);
        return copied ? {} : { error: 'Failed to copy' };
    };

    return (
        <div className="border border-border/50 rounded-lg overflow-hidden">
            <div className="flex items-center gap-2 px-3 py-2 bg-surface-light hover:bg-white/5">
                {isResolved && entries.length > 0 && (
                    expanded ? (
                        <ChevronDownIcon className="w-4 h-4 text-gray-500" />
                    ) : (
                        <ChevronRightIcon className="w-4 h-4 text-gray-500" />
                    )
                )}

                <SourceBadge type={type} />

                <ClickableEnvValue
                    title={isResolved ? 'Click to expand keys' : isStale ? 'Cannot resolve in stale context' : 'Click to resolve keys'}
                    disabled={isStale && !isResolved}
                    resolving={resolving}
                    className={isSecret ? 'bg-yellow-500/10 text-yellow-300 border-yellow-500/30' : 'text-gray-300'}
                    onCopy={type === 'unknown' ? undefined : resolveAndExpand}
                >
                    {sourceName}
                </ClickableEnvValue>

                {prefix && (
                    <span className="text-xs text-gray-500">prefix: {prefix}</span>
                )}

                {isResolved && (
                    <span className="text-xs text-gray-500">({entries.length} keys)</span>
                )}

                {hasError && (
                    <span className="text-xs text-red-400 flex items-center gap-1">
                        <ExclamationTriangleIcon className="w-3.5 h-3.5" />
                        {resolvedData.error}
                    </span>
                )}
            </div>

            {expanded && entries.length > 0 && (
                <div className="border-t border-border/50 px-3 py-2 bg-surface/50">
                    {entries.map(({ key, value }: { key: string; value: string }) => {
                        const displayValue = isSecret ? `$(secret:${sourceName}/${key})` : value;
                        return (
                            <div key={key} className="flex items-center gap-2 py-1 border-b border-border/20 last:border-b-0">
                                <code className="font-mono text-xs text-gray-200 shrink-0 max-w-[26rem] truncate" title={prefix + key}>
                                    {prefix}{key}
                                </code>
                                <div className="flex-1 min-w-0">
                                    <ClickableEnvValue
                                        title="Click to copy value"
                                        className={isSecret ? 'bg-yellow-500/10 text-yellow-300 border-yellow-500/30' : 'text-gray-300'}
                                        onCopy={copyEntry(value)}
                                    >
                                        {displayValueText(displayValue)}
                                    </ClickableEnvValue>
                                </div>
                            </div>
                        );
                    })}
                </div>
            )}
        </div>
    );
};

export default function EnvVarSection({
    env = [],
    envFrom = [],
    pod,
    namespace,
    isStale = false
}: { env?: any[]; envFrom?: any[]; pod: any; namespace: string; isStale?: boolean }) {
    const [expanded, setExpanded] = useState(false);
    const [searchTerm, setSearchTerm] = useState('');
    const [resolvedValues, setResolvedValues] = useState<Record<string, any>>({});
    const [resolvedEnvFrom, setResolvedEnvFrom] = useState<Record<string, any>>({});

    const totalCount = env.length + envFrom.length;
    const COLLAPSE_THRESHOLD = 50;

    const filteredEnv = useMemo(() => {
        if (!searchTerm) return env;
        const term = searchTerm.toLowerCase();
        return env.filter((e: any) =>
            e.name.toLowerCase().includes(term) ||
            e.value?.toLowerCase().includes(term)
        );
    }, [env, searchTerm]);

    const handleResolve = useCallback(async (type: string, name: string | null, key: string | null): Promise<ResolveResult> => {
        const cacheKey = `${cachePrefix(type)}:${name}/${key}`;
        const cached = resolvedValues[cacheKey];
        if (cached?.resolved) {
            return cached.error ? { error: cached.error } : { value: cached.value };
        }

        try {
            const data = type === 'configMap'
                ? await GetConfigMapData(namespace, name)
                : await GetSecretData(namespace, name);

            const textData = entryTextMap(data);
            if (textData && key && key in textData) {
                const value = textData[key as string];
                setResolvedValues(prev => ({
                    ...prev,
                    [cacheKey]: { resolved: true, value }
                }));
                return { value };
            }

            setResolvedValues(prev => ({
                ...prev,
                [cacheKey]: { resolved: true, value: null, error: 'Key not found' }
            }));
            return { error: 'Key not found' };
        } catch (err: any) {
            const error = err.message || 'Failed to resolve';
            setResolvedValues(prev => ({
                ...prev,
                [cacheKey]: { resolved: true, error }
            }));
            return { error };
        }
    }, [namespace, resolvedValues]);

    const handleResolveEnvFrom = useCallback(async (type: string, name: string | null): Promise<ResolveResult> => {
        const cacheKey = `${type}:${name}`;
        const cached = resolvedEnvFrom[cacheKey];
        if (cached?.resolved) {
            return cached.error ? { error: cached.error } : { entries: cached.entries || [] };
        }

        try {
            const data = type === 'configMap'
                ? await GetConfigMapData(namespace, name)
                : await GetSecretData(namespace, name);

            const textData = entryTextMap(data);
            if (textData) {
                const entries = Object.entries(textData).map(([key, value]: [string, any]) => ({ key, value }));
                setResolvedEnvFrom(prev => ({
                    ...prev,
                    [cacheKey]: { resolved: true, entries }
                }));
                return { entries };
            }

            setResolvedEnvFrom(prev => ({
                ...prev,
                [cacheKey]: { resolved: true, entries: [], error: 'Resource not found' }
            }));
            return { error: 'Resource not found' };
        } catch (err: any) {
            const error = err.message || 'Failed to resolve';
            setResolvedEnvFrom(prev => ({
                ...prev,
                [cacheKey]: { resolved: true, error }
            }));
            return { error };
        }
    }, [namespace, resolvedEnvFrom]);

    const enrichedEnv = useMemo(() => {
        return filteredEnv.map((e: any) => {
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
            <div className="flex items-center gap-2 mb-3">
                <span className="text-xs text-gray-500">
                    {env.length} variable{env.length !== 1 ? 's' : ''}
                    {envFrom.length > 0 && `, ${envFrom.length} envFrom`}
                </span>

                {env.length > 3 && (
                    <div className="relative">
                        <MagnifyingGlassIcon className="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-gray-500" />
                        <input
                            type="text"
                            value={searchTerm}
                            onChange={(e: any) => setSearchTerm(e.target.value)}
                            placeholder="Filter..."
                            className="pl-7 pr-2 py-1 text-xs bg-surface border border-border rounded w-32 focus:outline-none focus:border-primary"
                            autoComplete="off"
                            autoCorrect="off"
                            autoCapitalize="off"
                            spellCheck="false"
                        />
                    </div>
                )}
            </div>

            <div className="space-y-0">
                {displayEnv.map((e: any, idx: number) => (
                    <EnvVarItem
                        key={e.name || idx}
                        envVar={e}
                        pod={pod}
                        isStale={isStale}
                        resolvedValues={resolvedValues}
                        onResolve={handleResolve}
                    />
                ))}
            </div>

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

            {envFrom.length > 0 && (
                <div className="mt-3 space-y-2">
                    <div className="text-xs text-gray-500 mb-2">envFrom:</div>
                    {envFrom.map((ef: any, idx: number) => (
                        <EnvFromItem
                            key={ef.configMapRef?.name || ef.secretRef?.name || idx}
                            envFrom={ef}
                            isStale={isStale}
                            resolvedEnvFrom={resolvedEnvFrom}
                            onResolveEnvFrom={handleResolveEnvFrom}
                        />
                    ))}
                </div>
            )}
        </div>
    );
}
