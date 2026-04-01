import React from 'react';
import { PencilSquareIcon, ShareIcon, ClockIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, CopyableLabel, LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function LeaseDetails({ lease, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = lease?.metadata || {};
    const spec = lease?.spec || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-lease-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ClockIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="lease"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-lease-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ClockIcon,
            content: (
                <DependencyGraph
                    resourceType="lease"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const formatDuration = (seconds: any) => {
        if (!seconds) return '-';
        if (seconds < 60) return `${seconds}s`;
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
        return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
    };

    const formatTimestamp = (timestamp: any) => {
        if (!timestamp) return '-';
        const date = new Date(timestamp);
        return date.toLocaleString();
    };

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
                {/* Leader Election Info */}
                {spec.holderIdentity && (
                    <DetailSection title="Leader Election">
                        <div className="bg-background-dark rounded border border-border p-3">
                            <div className="flex items-center gap-2">
                                <div className="w-2 h-2 bg-green-400 rounded-full animate-pulse"></div>
                                <span className="text-sm text-gray-300">Current leader:</span>
                                <CopyableLabel value={spec.holderIdentity} />
                            </div>
                            {spec.leaseTransitions !== undefined && spec.leaseTransitions > 0 && (
                                <p className="text-xs text-gray-500 mt-2">
                                    Leadership has changed {spec.leaseTransitions} time(s)
                                </p>
                            )}
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Lease Duration" value={formatDuration(spec.leaseDurationSeconds)} />
                    <DetailRow label="Lease Transitions" value={spec.leaseTransitions ?? '-'} />
                    <DetailRow label="Created">
                        <span title={metadata.creationTimestamp}>
                            {formatAge(metadata.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={metadata.uid?.substring(0, 8) + '...'} copyValue={metadata.uid} />
                    </DetailRow>
                </DetailSection>

                {/* Timing */}
                <DetailSection title="Timing">
                    <DetailRow label="Acquire Time" value={formatTimestamp(spec.acquireTime)} />
                    <DetailRow label="Renew Time" value={formatTimestamp(spec.renewTime)} />
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
