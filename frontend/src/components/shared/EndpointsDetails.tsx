import React from 'react';
import { PencilSquareIcon, ShareIcon, QueueListIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function EndpointsDetails({ endpoints, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = endpoints?.metadata || {};
    const subsets = endpoints?.subsets || [];

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-endpoints-${endpoints.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: QueueListIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="endpoints"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-endpoints-${endpoints.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: QueueListIcon,
            content: (
                <DependencyGraph
                    resourceType="endpoints"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Subsets', value: subsets.length.toString() },
    ];

    const getAllAddresses = () => {
        const ready = [];
        const notReady = [];
        subsets.forEach(subset => {
            (subset.addresses || []).forEach(addr => {
                ready.push({ ...addr, ports: subset.ports });
            });
            (subset.notReadyAddresses || []).forEach(addr => {
                notReady.push({ ...addr, ports: subset.ports });
            });
        });
        return { ready, notReady };
    };

    const { ready, notReady } = getAllAddresses();

    const formatPorts = (ports) => {
        if (!ports || ports.length === 0) return '-';
        return ports.map(p => `${p.port}/${p.protocol || 'TCP'}`).join(', ');
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

                {/* Ready Addresses */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">
                        Ready Addresses ({ready.length})
                    </h3>
                    {ready.length === 0 ? (
                        <p className="text-sm text-gray-500">No ready addresses</p>
                    ) : (
                        <div className="space-y-2">
                            {ready.map((addr, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="flex items-center justify-between">
                                        <div>
                                            <span className="text-sm text-green-400 font-mono">{addr.ip}</span>
                                            {addr.hostname && (
                                                <span className="text-xs text-gray-500 ml-2">({addr.hostname})</span>
                                            )}
                                        </div>
                                        <span className="text-xs text-gray-400">{formatPorts(addr.ports)}</span>
                                    </div>
                                    {addr.targetRef && (
                                        <div className="text-xs text-gray-500 mt-1">
                                            {addr.targetRef.kind}: {addr.targetRef.name}
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    )}
                </div>

                {/* Not Ready Addresses */}
                {notReady.length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">
                            Not Ready Addresses ({notReady.length})
                        </h3>
                        <div className="space-y-2">
                            {notReady.map((addr, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3 border-l-2 border-yellow-500">
                                    <div className="flex items-center justify-between">
                                        <div>
                                            <span className="text-sm text-yellow-400 font-mono">{addr.ip}</span>
                                            {addr.hostname && (
                                                <span className="text-xs text-gray-500 ml-2">({addr.hostname})</span>
                                            )}
                                        </div>
                                        <span className="text-xs text-gray-400">{formatPorts(addr.ports)}</span>
                                    </div>
                                    {addr.targetRef && (
                                        <div className="text-xs text-gray-500 mt-1">
                                            {addr.targetRef.kind}: {addr.targetRef.name}
                                        </div>
                                    )}
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
