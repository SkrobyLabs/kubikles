import React from 'react';
import { PencilSquareIcon, ShareIcon, QueueListIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function EndpointsDetails({ endpoints, tabContext = '' }: { endpoints: any; tabContext?: string }) {
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

    const getAllAddresses = () => {
        const ready: any[] = [];
        const notReady: any[] = [];
        subsets.forEach((subset: any) => {
            (subset.addresses || []).forEach((addr: any) => {
                ready.push({ ...addr, ports: subset.ports });
            });
            (subset.notReadyAddresses || []).forEach((addr: any) => {
                notReady.push({ ...addr, ports: subset.ports });
            });
        });
        return { ready, notReady };
    };

    const { ready, notReady } = getAllAddresses();

    const formatPorts = (ports: any) => {
        if (!ports || ports.length === 0) return '-';
        return ports.map((p: any) => `${p.port}/${p.protocol || 'TCP'}`).join(', ');
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
                            disabled={!!isStale}
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
                {/* Ready Addresses */}
                <DetailSection title={`Ready Addresses (${ready.length})`}>
                    {ready.length === 0 ? (
                        <span className="text-gray-500">No ready addresses</span>
                    ) : (
                        <div className="space-y-2">
                            {ready.map((addr: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3">
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
                </DetailSection>

                {/* Not Ready Addresses */}
                {notReady.length > 0 && (
                    <DetailSection title={`Not Ready Addresses (${notReady.length})`}>
                        <div className="space-y-2">
                            {notReady.map((addr: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3 border-l-2 border-l-yellow-500">
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
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Subsets" value={subsets.length} />
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
