import React from 'react';
import { PencilSquareIcon, ShareIcon, BoltIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function PriorityClassDetails({ priorityClass, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = priorityClass?.metadata || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-priorityclass-${name}`;
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
        const tabId = `deps-priorityclass-${name}`;
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

    const formatValue = (value: any) => {
        if (value >= 1000000000) return `${(value / 1000000000).toFixed(1)}B (${value.toLocaleString()})`;
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M (${value.toLocaleString()})`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K (${value.toLocaleString()})`;
        return value?.toLocaleString() || '0';
    };

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
                {/* Description */}
                {priorityClass.description && (
                    <DetailSection title="Description">
                        <p className="text-sm text-gray-300 bg-background-dark rounded border border-border p-3">
                            {priorityClass.description}
                        </p>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Value" value={formatValue(priorityClass.value)} />
                    <DetailRow label="Global Default" value={priorityClass.globalDefault ? 'Yes' : 'No'} />
                    <DetailRow label="Preemption Policy" value={priorityClass.preemptionPolicy || 'PreemptLowerPriority'} />
                    <DetailRow label="Created">
                        <span title={metadata.creationTimestamp}>
                            {formatAge(metadata.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={metadata.uid?.substring(0, 8) + '...'} copyValue={metadata.uid} />
                    </DetailRow>
                </DetailSection>

                {/* Labels */}
                <DetailSection title="Labels">
                    <LabelsDisplay labels={metadata.labels} />
                </DetailSection>

                {/* Annotations */}
                <DetailSection title="Annotations">
                    <AnnotationsDisplay annotations={metadata.annotations} />
                </DetailSection>
            </div>
        </div>
    );
}
