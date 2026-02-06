import React from 'react';
import { PencilSquareIcon, ShareIcon, ServerIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function CSINodeDetails({ csiNode, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = csiNode?.metadata || {};
    const spec = csiNode?.spec || {};
    const drivers = spec.drivers || [];

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;

    const handleEditYaml = () => {
        const tabId = `yaml-csinode-${csiNode.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ServerIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="csinode"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-csinode-${csiNode.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ServerIcon,
            content: (
                <DependencyGraph
                    resourceType="csinode"
                    namespace=""
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Driver Count', value: drivers.length },
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
                                <dd className="text-sm text-gray-200 mt-0.5">{value ?? '-'}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* CSI Drivers */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">CSI Drivers</h3>
                    {drivers.length === 0 ? (
                        <p className="text-sm text-gray-500">No CSI drivers registered on this node</p>
                    ) : (
                        <div className="space-y-3">
                            {drivers.map((driver: any, idx: number) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="flex items-center justify-between mb-2">
                                        <span className="text-sm font-medium text-gray-200">{driver.name}</span>
                                        {driver.allocatable?.count !== undefined && (
                                            <span className="text-xs bg-blue-500/20 text-blue-400 px-2 py-0.5 rounded">
                                                {driver.allocatable.count} allocatable
                                            </span>
                                        )}
                                    </div>

                                    {driver.nodeID && (
                                        <div className="text-xs text-gray-500 mb-1">
                                            Node ID: <span className="text-gray-400">{driver.nodeID}</span>
                                        </div>
                                    )}

                                    {driver.topologyKeys && driver.topologyKeys.length > 0 && (
                                        <div className="mt-2">
                                            <div className="text-xs text-gray-500 mb-1">Topology Keys:</div>
                                            <div className="flex flex-wrap gap-1">
                                                {driver.topologyKeys.map((key: any, keyIdx: number) => (
                                                    <span
                                                        key={keyIdx}
                                                        className="text-xs px-1.5 py-0.5 bg-gray-700 text-gray-300 rounded"
                                                    >
                                                        {key}
                                                    </span>
                                                ))}
                                            </div>
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    )}
                </div>

                {/* Owner References */}
                {metadata.ownerReferences && metadata.ownerReferences.length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Owner References</h3>
                        <div className="space-y-2">
                            {metadata.ownerReferences.map((ref: any, idx: number) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="text-sm text-gray-300">
                                        {ref.kind}: {ref.name}
                                    </div>
                                    <div className="text-xs text-gray-500 mt-1">
                                        API Version: {ref.apiVersion}
                                    </div>
                                </div>
                            ))}
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
