import React from 'react';
import { PencilSquareIcon, ShareIcon, AdjustmentsHorizontalIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context';
import { useUI } from '../../context';
import { formatAge } from '../../utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function ResourceQuotaDetails({ resourceQuota, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = resourceQuota?.metadata || {};
    const spec = resourceQuota?.spec || {};
    const status = resourceQuota?.status || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-resourcequota-${resourceQuota.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: AdjustmentsHorizontalIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="resourcequota"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-resourcequota-${resourceQuota.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: AdjustmentsHorizontalIcon,
            content: (
                <DependencyGraph
                    resourceType="resourcequota"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const hard = spec.hard || {};
    const used = status.used || {};

    const getQuotaItems = () => {
        const items = [];
        for (const [key, value] of Object.entries(hard)) {
            const usedValue = used[key] || '0';
            items.push({
                resource: key,
                hard: value,
                used: usedValue,
            });
        }
        return items;
    };

    const quotaItems = getQuotaItems();

    const parseQuantity = (value) => {
        if (typeof value === 'number') return value;
        if (typeof value !== 'string') return 0;
        const num = parseFloat(value);
        if (isNaN(num)) return 0;
        if (value.endsWith('Ki')) return num * 1024;
        if (value.endsWith('Mi')) return num * 1024 * 1024;
        if (value.endsWith('Gi')) return num * 1024 * 1024 * 1024;
        if (value.endsWith('Ti')) return num * 1024 * 1024 * 1024 * 1024;
        if (value.endsWith('m')) return num / 1000;
        return num;
    };

    const getUsagePercent = (usedVal, hardVal) => {
        const usedNum = parseQuantity(usedVal);
        const hardNum = parseQuantity(hardVal);
        if (hardNum === 0) return 0;
        return Math.min(100, Math.round((usedNum / hardNum) * 100));
    };

    const getUsageColor = (percent) => {
        if (percent >= 90) return 'bg-red-500';
        if (percent >= 70) return 'bg-yellow-500';
        return 'bg-green-500';
    };

    const scopes = spec.scopes || [];
    const scopeSelector = spec.scopeSelector;

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Resources', value: quotaItems.length },
    ];

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {namespace}/{name}
                    </div>
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        <button
                            onClick={handleEditYaml}
                            className={`p-1.5 rounded transition-colors ${isStale ? 'text-gray-600 cursor-not-allowed' : 'text-gray-400 hover:text-white hover:bg-white/10'}`}
                            title="Edit YAML"
                            disabled={isStale}
                        >
                            <PencilSquareIcon className="w-4 h-4" />
                        </button>
                        <button
                            onClick={handleShowDependencies}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Dependencies"
                        >
                            <ShareIcon className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            <div className="h-full overflow-auto p-4">
            <div className="space-y-6">
                {/* Basic Info */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Basic Information</h3>
                    <div className="grid grid-cols-2 gap-4">
                        {basicInfo.map(({ label, value }) => (
                            <div key={label}>
                                <dt className="text-xs text-gray-500">{label}</dt>
                                <dd className="text-sm text-gray-200 mt-0.5">{value ?? '-'}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Resource Usage */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Resource Usage</h3>
                    {quotaItems.length === 0 ? (
                        <p className="text-sm text-gray-500">No resources defined</p>
                    ) : (
                        <div className="space-y-3">
                            {quotaItems.map(({ resource, hard, used }) => {
                                const percent = getUsagePercent(used, hard);
                                return (
                                    <div key={resource} className="bg-gray-800/50 rounded-lg p-3">
                                        <div className="flex justify-between items-center mb-2">
                                            <span className="text-sm text-gray-300">{resource}</span>
                                            <span className="text-sm text-gray-400">{used} / {hard}</span>
                                        </div>
                                        <div className="w-full bg-gray-700 rounded-full h-2">
                                            <div
                                                className={`h-2 rounded-full ${getUsageColor(percent)}`}
                                                style={{ width: `${percent}%` }}
                                            />
                                        </div>
                                        <div className="text-right text-xs text-gray-500 mt-1">{percent}%</div>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>

                {/* Scopes */}
                {scopes.length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Scopes</h3>
                        <div className="flex flex-wrap gap-2">
                            {scopes.map((scope) => (
                                <span
                                    key={scope}
                                    className="inline-flex items-center px-2 py-1 rounded text-xs bg-blue-900/50 text-blue-300"
                                >
                                    {scope}
                                </span>
                            ))}
                        </div>
                    </div>
                )}

                {/* Scope Selector */}
                {scopeSelector && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Scope Selector</h3>
                        <div className="bg-gray-800/50 rounded-lg p-3">
                            <pre className="text-xs text-gray-300 overflow-auto">
                                {JSON.stringify(scopeSelector, null, 2)}
                            </pre>
                        </div>
                    </div>
                )}

                {/* Labels */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Labels</h3>
                    <LabelsDisplay labels={metadata.labels} />
                </div>

                {/* Annotations */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Annotations</h3>
                    <AnnotationsDisplay annotations={metadata.annotations} />
                </div>
            </div>
            </div>
        </div>
    );
}
