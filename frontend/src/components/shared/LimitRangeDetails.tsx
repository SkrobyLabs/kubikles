import React from 'react';
import { PencilSquareIcon, ShareIcon, ArrowsPointingOutIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function LimitRangeDetails({ limitRange, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = limitRange?.metadata || {};
    const spec = limitRange?.spec || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-limitrange-${limitRange.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ArrowsPointingOutIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="limitrange"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-limitrange-${limitRange.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ArrowsPointingOutIcon,
            content: (
                <DependencyGraph
                    resourceType="limitrange"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const limits = spec.limits || [];

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
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
                {/* Limits */}
                <DetailSection title="Limits">
                    {limits.length === 0 ? (
                        <span className="text-gray-500">No limits defined</span>
                    ) : (
                        <div className="space-y-4">
                            {limits.map((limit: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-4">
                                    <div className="flex items-center gap-2 mb-3">
                                        <span className="text-sm font-medium text-gray-200">{limit.type}</span>
                                        <span className="text-xs text-gray-500 bg-gray-700 px-2 py-0.5 rounded">
                                            Type
                                        </span>
                                    </div>
                                    <div className="overflow-x-auto">
                                        <table className="w-full text-sm">
                                            <thead>
                                                <tr className="text-left text-xs text-gray-500 border-b border-border">
                                                    <th className="pb-2 pr-4">Resource</th>
                                                    <th className="pb-2 pr-4">Min</th>
                                                    <th className="pb-2 pr-4">Max</th>
                                                    <th className="pb-2 pr-4">Default</th>
                                                    <th className="pb-2 pr-4">Default Request</th>
                                                    <th className="pb-2">Max Limit/Request Ratio</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                {['cpu', 'memory', 'storage', 'ephemeral-storage'].map((resource) => {
                                                    const min = limit.min?.[resource];
                                                    const max = limit.max?.[resource];
                                                    const defaultVal = limit.default?.[resource];
                                                    const defaultRequest = limit.defaultRequest?.[resource];
                                                    const ratio = limit.maxLimitRequestRatio?.[resource];

                                                    if (!min && !max && !defaultVal && !defaultRequest && !ratio) {
                                                        return null;
                                                    }

                                                    return (
                                                        <tr key={resource} className="border-b border-border/50">
                                                            <td className="py-2 pr-4 text-gray-300">{resource}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{min || '-'}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{max || '-'}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{defaultVal || '-'}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{defaultRequest || '-'}</td>
                                                            <td className="py-2 text-gray-400">{ratio || '-'}</td>
                                                        </tr>
                                                    );
                                                })}
                                            </tbody>
                                        </table>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </DetailSection>

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Limit Types" value={limits.length} />
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
