import React from 'react';
import { PencilSquareIcon, ShareIcon, BoltIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context';
import { useUI } from '../../context';
import { formatAge } from '../../utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function PriorityClassDetails({ priorityClass, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = priorityClass?.metadata || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-priorityclass-${priorityClass.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: BoltIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="priorityclass"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-priorityclass-${priorityClass.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: BoltIcon,
            content: (
                <DependencyGraph
                    resourceType="priorityclass"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const formatValue = (value) => {
        if (value >= 1000000000) return `${(value / 1000000000).toFixed(1)}B (${value.toLocaleString()})`;
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M (${value.toLocaleString()})`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K (${value.toLocaleString()})`;
        return value?.toLocaleString() || '0';
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Value', value: formatValue(priorityClass.value) },
        { label: 'Global Default', value: priorityClass.globalDefault ? 'Yes' : 'No' },
        { label: 'Preemption Policy', value: priorityClass.preemptionPolicy || 'PreemptLowerPriority' },
    ];

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {name}
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
                                <dd className="text-sm text-gray-200 mt-0.5">{value || '-'}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Description */}
                {priorityClass.description && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Description</h3>
                        <p className="text-sm text-gray-300 bg-gray-800/50 rounded-lg p-3">
                            {priorityClass.description}
                        </p>
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
