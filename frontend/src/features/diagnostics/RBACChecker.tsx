import React, { useState, useCallback, useMemo } from 'react';
import { CheckRBACAccess } from 'wailsjs/go/main/App';
import { useK8s } from '~/context';
import Tooltip from '~/components/shared/Tooltip';
import SearchSelect from '~/components/shared/SearchSelect';
import {
    ShieldCheckIcon,
    ShieldExclamationIcon,
    ArrowPathIcon,
    XMarkIcon,
    CheckCircleIcon,
    XCircleIcon,
    QuestionMarkCircleIcon,
    UserIcon,
    UserGroupIcon,
    CogIcon,
    LinkIcon
} from '@heroicons/react/24/outline';

const SUBJECT_KINDS = [
    { value: 'ServiceAccount', label: 'ServiceAccount', icon: CogIcon },
    { value: 'User', label: 'User', icon: UserIcon },
    { value: 'Group', label: 'Group', icon: UserGroupIcon }
];

const COMMON_VERBS = ['get', 'list', 'watch', 'create', 'update', 'patch', 'delete', 'deletecollection'];

const COMMON_RESOURCES = [
    { value: 'pods', label: 'pods', group: '' },
    { value: 'pods/log', label: 'pods/log', group: '' },
    { value: 'pods/exec', label: 'pods/exec', group: '' },
    { value: 'pods/portforward', label: 'pods/portforward', group: '' },
    { value: 'deployments', label: 'deployments', group: 'apps' },
    { value: 'statefulsets', label: 'statefulsets', group: 'apps' },
    { value: 'daemonsets', label: 'daemonsets', group: 'apps' },
    { value: 'replicasets', label: 'replicasets', group: 'apps' },
    { value: 'services', label: 'services', group: '' },
    { value: 'configmaps', label: 'configmaps', group: '' },
    { value: 'secrets', label: 'secrets', group: '' },
    { value: 'persistentvolumeclaims', label: 'persistentvolumeclaims', group: '' },
    { value: 'jobs', label: 'jobs', group: 'batch' },
    { value: 'cronjobs', label: 'cronjobs', group: 'batch' },
    { value: 'ingresses', label: 'ingresses', group: 'networking.k8s.io' },
    { value: 'networkpolicies', label: 'networkpolicies', group: 'networking.k8s.io' },
    { value: 'roles', label: 'roles', group: 'rbac.authorization.k8s.io' },
    { value: 'rolebindings', label: 'rolebindings', group: 'rbac.authorization.k8s.io' },
    { value: 'clusterroles', label: 'clusterroles', group: 'rbac.authorization.k8s.io' },
    { value: 'clusterrolebindings', label: 'clusterrolebindings', group: 'rbac.authorization.k8s.io' }
];

function ChainLink({ link }) {
    const isBinding = link.kind.includes('Binding');
    const bgColor = link.grants
        ? 'bg-green-900/20 border-green-800/50'
        : 'bg-surface border-border';

    return (
        <div className={`p-3 rounded border ${bgColor}`}>
            <div className="flex items-center gap-2 mb-1">
                <LinkIcon className="h-4 w-4 text-gray-500" />
                <span className="text-sm font-medium text-text">
                    {link.kind}
                </span>
                {link.grants && (
                    <CheckCircleIcon className="h-4 w-4 text-green-400" />
                )}
            </div>
            <div className="text-xs text-gray-400 space-y-0.5">
                <div>
                    <span className="text-gray-500">Name:</span>{' '}
                    <span className="text-text font-mono">{link.name}</span>
                </div>
                {link.namespace && (
                    <div>
                        <span className="text-gray-500">Namespace:</span>{' '}
                        <span className="text-text font-mono">{link.namespace}</span>
                    </div>
                )}
                {link.rule && (
                    <div className="mt-1 p-1.5 bg-background rounded text-gray-400 font-mono text-xs">
                        {link.rule}
                    </div>
                )}
            </div>
        </div>
    );
}

export default function RBACChecker({
    initialSubject = {},
    initialAction = {},
    onClose
}) {
    const { currentNamespace } = useK8s();

    // Form state
    const [subjectKind, setSubjectKind] = useState(initialSubject.kind || 'ServiceAccount');
    const [subjectName, setSubjectName] = useState(initialSubject.name || '');
    const [subjectNamespace, setSubjectNamespace] = useState(initialSubject.namespace || currentNamespace || 'default');
    const [verb, setVerb] = useState(initialAction.verb || 'get');
    const [resource, setResource] = useState(initialAction.resource || 'pods');
    const [resourceName, setResourceName] = useState(initialAction.resourceName || '');
    const [namespace, setNamespace] = useState(initialAction.namespace || currentNamespace || 'default');
    const [apiGroup, setApiGroup] = useState(initialAction.apiGroup || '');
    const [customResource, setCustomResource] = useState(false);

    // Result state
    const [result, setResult] = useState(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    // Handle resource selection
    const handleResourceSelect = useCallback((value) => {
        const found = COMMON_RESOURCES.find(r => r.value === value);
        setResource(value);
        if (found) {
            setApiGroup(found.group);
        }
    }, []);

    // Perform the check
    const checkAccess = useCallback(async () => {
        if (!subjectName) {
            setError('Subject name is required');
            return;
        }
        if (!resource) {
            setError('Resource is required');
            return;
        }

        setLoading(true);
        setError(null);

        try {
            const checkResult = await CheckRBACAccess(
                subjectKind,
                subjectName,
                subjectKind === 'ServiceAccount' ? subjectNamespace : '',
                verb,
                resource,
                resourceName,
                namespace,
                apiGroup
            );
            setResult(checkResult);
        } catch (err) {
            setError(err.message || 'Failed to check RBAC access');
            setResult(null);
        } finally {
            setLoading(false);
        }
    }, [subjectKind, subjectName, subjectNamespace, verb, resource, resourceName, namespace, apiGroup]);

    return (
        <div className="h-full flex flex-col bg-background text-text">
            {/* Header */}
            <div className="flex-shrink-0 border-b border-border p-4">
                <div className="flex items-center justify-between mb-4">
                    <h2 className="text-lg font-semibold flex items-center gap-2">
                        <ShieldCheckIcon className="h-5 w-5 text-amber-400" />
                        RBAC Checker
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

                {/* Subject Selection */}
                <div className="mb-4">
                    <h3 className="text-sm font-medium text-text mb-2 flex items-center gap-2">
                        <QuestionMarkCircleIcon className="h-4 w-4 text-gray-500" />
                        Who is checking?
                    </h3>
                    <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
                        <div>
                            <label className="block text-xs text-gray-400 mb-1">Subject Type</label>
                            <div className="flex gap-1">
                                {SUBJECT_KINDS.map(sk => (
                                    <button
                                        key={sk.value}
                                        onClick={() => setSubjectKind(sk.value)}
                                        className={`flex-1 px-2 py-1.5 rounded text-xs flex items-center justify-center gap-1 transition-colors ${
                                            subjectKind === sk.value
                                                ? 'bg-primary text-white'
                                                : 'bg-surface text-gray-300 hover:bg-surface-light'
                                        }`}
                                    >
                                        <sk.icon className="h-3.5 w-3.5" />
                                        {sk.label}
                                    </button>
                                ))}
                            </div>
                        </div>
                        <div>
                            <label className="block text-xs text-gray-400 mb-1">Name</label>
                            <input
                                type="text"
                                value={subjectName}
                                onChange={(e) => setSubjectName(e.target.value)}
                                className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                                placeholder={subjectKind === 'ServiceAccount' ? 'default' : 'admin'}
                                autoComplete="off"
                            />
                        </div>
                        {subjectKind === 'ServiceAccount' && (
                            <div>
                                <label className="block text-xs text-gray-400 mb-1">SA Namespace</label>
                                <input
                                    type="text"
                                    value={subjectNamespace}
                                    onChange={(e) => setSubjectNamespace(e.target.value)}
                                    className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                                    placeholder="default"
                                    autoComplete="off"
                                />
                            </div>
                        )}
                    </div>
                </div>

                {/* Action Selection */}
                <div className="mb-4">
                    <h3 className="text-sm font-medium text-text mb-2 flex items-center gap-2">
                        <CogIcon className="h-4 w-4 text-gray-500" />
                        What action?
                    </h3>
                    <div className="grid grid-cols-1 md:grid-cols-4 gap-3">
                        <div>
                            <label className="block text-xs text-gray-400 mb-1">Verb</label>
                            <SearchSelect
                                options={COMMON_VERBS}
                                value={verb}
                                onChange={setVerb}
                                placeholder="Select verb..."
                                preserveOrder={true}
                            />
                        </div>
                        <div>
                            <label className="block text-xs text-gray-400 mb-1">
                                Resource
                                <button
                                    onClick={() => setCustomResource(!customResource)}
                                    className="ml-2 text-primary text-xs hover:underline"
                                >
                                    {customResource ? '(use dropdown)' : '(custom)'}
                                </button>
                            </label>
                            {customResource ? (
                                <input
                                    type="text"
                                    value={resource}
                                    onChange={(e) => setResource(e.target.value)}
                                    className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                                    placeholder="pods"
                                    autoComplete="off"
                                />
                            ) : (
                                <SearchSelect
                                    options={COMMON_RESOURCES}
                                    value={resource}
                                    onChange={handleResourceSelect}
                                    placeholder="Select resource..."
                                    getOptionValue={(r) => r.value}
                                    getOptionLabel={(r) => r.group ? `${r.label} (${r.group})` : r.label}
                                    preserveOrder={true}
                                />
                            )}
                        </div>
                        <div>
                            <label className="block text-xs text-gray-400 mb-1">Target Namespace</label>
                            <input
                                type="text"
                                value={namespace}
                                onChange={(e) => setNamespace(e.target.value)}
                                className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                                placeholder="default (empty=cluster-wide)"
                                autoComplete="off"
                            />
                        </div>
                        <div>
                            <label className="block text-xs text-gray-400 mb-1">Resource Name (optional)</label>
                            <input
                                type="text"
                                value={resourceName}
                                onChange={(e) => setResourceName(e.target.value)}
                                className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                                placeholder="specific-pod"
                                autoComplete="off"
                            />
                        </div>
                    </div>
                    {customResource && (
                        <div className="mt-2">
                            <label className="block text-xs text-gray-400 mb-1">API Group</label>
                            <input
                                type="text"
                                value={apiGroup}
                                onChange={(e) => setApiGroup(e.target.value)}
                                className="w-48 px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary/50"
                                placeholder="apps (empty=core)"
                                autoComplete="off"
                            />
                        </div>
                    )}
                </div>

                <button
                    onClick={checkAccess}
                    disabled={loading || !subjectName || !resource}
                    className="px-4 py-2 bg-primary hover:bg-primary/80 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium text-white flex items-center gap-2 transition-colors"
                >
                    {loading ? (
                        <ArrowPathIcon className="h-4 w-4 animate-spin" />
                    ) : (
                        <ShieldCheckIcon className="h-4 w-4" />
                    )}
                    Check Permission
                </button>
            </div>

            {/* Error Display */}
            {error && (
                <div className="flex-shrink-0 p-4 bg-red-900/20 border-b border-red-800/50 text-red-400 text-sm">
                    {error}
                </div>
            )}

            {/* Result Display */}
            <div className="flex-1 min-h-0 overflow-auto p-4">
                {loading ? (
                    <div className="flex items-center justify-center h-32">
                        <ArrowPathIcon className="h-8 w-8 text-gray-500 animate-spin" />
                    </div>
                ) : !result ? (
                    <div className="flex flex-col items-center justify-center h-32 text-gray-500">
                        <ShieldCheckIcon className="h-12 w-12 mb-2 opacity-50" />
                        <p>Configure a subject and action to check permissions</p>
                    </div>
                ) : (
                    <div className="space-y-4">
                        {/* Result Banner */}
                        <div className={`p-4 rounded-lg flex items-center gap-4 ${
                            result.allowed
                                ? 'bg-green-900/20 border border-green-800/50'
                                : 'bg-red-900/20 border border-red-800/50'
                        }`}>
                            {result.allowed ? (
                                <CheckCircleIcon className="h-10 w-10 text-green-400 flex-shrink-0" />
                            ) : (
                                <XCircleIcon className="h-10 w-10 text-red-400 flex-shrink-0" />
                            )}
                            <div>
                                <h3 className={`text-lg font-semibold ${
                                    result.allowed ? 'text-green-400' : 'text-red-400'
                                }`}>
                                    {result.allowed ? 'Access Allowed' : 'Access Denied'}
                                </h3>
                                <p className="text-sm text-gray-400 mt-1">
                                    {result.reason}
                                </p>
                            </div>
                        </div>

                        {/* Query Summary */}
                        <div className="p-3 bg-surface rounded border border-border">
                            <h4 className="text-sm font-medium text-text mb-2">Query</h4>
                            <div className="text-xs font-mono text-gray-400">
                                Can{' '}
                                <span className="text-blue-400">{subjectKind}/{subjectName}</span>
                                {subjectKind === 'ServiceAccount' && (
                                    <span className="text-gray-500"> (ns: {subjectNamespace})</span>
                                )}
                                {' '}<span className="text-amber-400">{verb}</span>{' '}
                                <span className="text-green-400">{resource}</span>
                                {resourceName && (
                                    <span className="text-purple-400">/{resourceName}</span>
                                )}
                                {apiGroup && (
                                    <span className="text-gray-500"> (group: {apiGroup})</span>
                                )}
                                {namespace ? (
                                    <span className="text-gray-500"> in namespace <span className="text-cyan-400">{namespace}</span></span>
                                ) : (
                                    <span className="text-gray-500"> (cluster-wide)</span>
                                )}
                                ?
                            </div>
                        </div>

                        {/* RBAC Chain */}
                        {result.chain && result.chain.length > 0 && (
                            <div>
                                <h4 className="text-sm font-medium text-text mb-2 flex items-center gap-2">
                                    <LinkIcon className="h-4 w-4 text-gray-500" />
                                    Authorization Chain
                                </h4>
                                <div className="space-y-2">
                                    {result.chain.map((link, index) => (
                                        <ChainLink key={index} link={link} />
                                    ))}
                                </div>
                            </div>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}
