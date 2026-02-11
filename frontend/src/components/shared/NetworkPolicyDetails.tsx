import React from 'react';
import { PencilSquareIcon, ShareIcon, ShieldCheckIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function NetworkPolicyDetails({ networkPolicy, tabContext = '' }: { networkPolicy: any; tabContext?: string }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = networkPolicy?.metadata || {};
    const spec = networkPolicy?.spec || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-networkpolicy-${networkPolicy.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldCheckIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="networkpolicy"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-networkpolicy-${networkPolicy.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldCheckIcon,
            content: (
                <DependencyGraph
                    resourceType="networkpolicy"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const formatLabelSelector = (selector: any) => {
        if (!selector || Object.keys(selector.matchLabels || {}).length === 0) {
            return 'All Pods';
        }
        return Object.entries(selector.matchLabels || {})
            .map(([k, v]) => `${k}=${v}`)
            .join(', ');
    };

    const formatPort = (port: any) => {
        if (!port) return 'All';
        const protocol = port.protocol || 'TCP';
        const portNum = port.port || 'All';
        return `${portNum}/${protocol}`;
    };

    const formatPeer = (peer: any) => {
        if (!peer) return 'Any';
        const parts = [];
        if (peer.ipBlock) {
            parts.push(`CIDR: ${peer.ipBlock.cidr}`);
            if (peer.ipBlock.except?.length > 0) {
                parts.push(`except: ${peer.ipBlock.except.join(', ')}`);
            }
        }
        if (peer.namespaceSelector) {
            const labels = Object.entries(peer.namespaceSelector.matchLabels || {})
                .map(([k, v]) => `${k}=${v}`)
                .join(', ');
            parts.push(`Namespaces: ${labels || 'All'}`);
        }
        if (peer.podSelector) {
            const labels = Object.entries(peer.podSelector.matchLabels || {})
                .map(([k, v]) => `${k}=${v}`)
                .join(', ');
            parts.push(`Pods: ${labels || 'All in namespace'}`);
        }
        return parts.length > 0 ? parts.join('; ') : 'Any';
    };

    const ingressRules = spec.ingress || [];
    const egressRules = spec.egress || [];

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
                {/* Ingress Rules */}
                <DetailSection title={`Ingress Rules (${ingressRules.length})`}>
                    {ingressRules.length === 0 ? (
                        <span className="text-gray-500">
                            {spec.policyTypes?.includes('Ingress')
                                ? 'No ingress rules (all ingress denied)'
                                : 'Ingress not restricted'}
                        </span>
                    ) : (
                        <div className="space-y-3">
                            {ingressRules.map((rule: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3">
                                    <div className="text-xs text-gray-400 mb-2">Rule {idx + 1}</div>
                                    <div className="space-y-2">
                                        <div>
                                            <span className="text-xs text-gray-500">From:</span>
                                            <div className="text-sm text-gray-300 ml-2">
                                                {rule.from?.length > 0
                                                    ? rule.from.map((peer: any, i: number) => (
                                                        <div key={i}>{formatPeer(peer)}</div>
                                                    ))
                                                    : 'Any source'}
                                            </div>
                                        </div>
                                        <div>
                                            <span className="text-xs text-gray-500">Ports:</span>
                                            <div className="text-sm text-gray-300 ml-2">
                                                {rule.ports?.length > 0
                                                    ? rule.ports.map(formatPort).join(', ')
                                                    : 'All ports'}
                                            </div>
                                        </div>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </DetailSection>

                {/* Egress Rules */}
                <DetailSection title={`Egress Rules (${egressRules.length})`}>
                    {egressRules.length === 0 ? (
                        <span className="text-gray-500">
                            {spec.policyTypes?.includes('Egress')
                                ? 'No egress rules (all egress denied)'
                                : 'Egress not restricted'}
                        </span>
                    ) : (
                        <div className="space-y-3">
                            {egressRules.map((rule: any, idx: number) => (
                                <div key={idx} className="bg-background-dark rounded border border-border p-3">
                                    <div className="text-xs text-gray-400 mb-2">Rule {idx + 1}</div>
                                    <div className="space-y-2">
                                        <div>
                                            <span className="text-xs text-gray-500">To:</span>
                                            <div className="text-sm text-gray-300 ml-2">
                                                {rule.to?.length > 0
                                                    ? rule.to.map((peer: any, i: number) => (
                                                        <div key={i}>{formatPeer(peer)}</div>
                                                    ))
                                                    : 'Any destination'}
                                            </div>
                                        </div>
                                        <div>
                                            <span className="text-xs text-gray-500">Ports:</span>
                                            <div className="text-sm text-gray-300 ml-2">
                                                {rule.ports?.length > 0
                                                    ? rule.ports.map(formatPort).join(', ')
                                                    : 'All ports'}
                                            </div>
                                        </div>
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
                    <DetailRow label="Pod Selector" value={formatLabelSelector(spec.podSelector)} />
                    <DetailRow label="Policy Types" value={spec.policyTypes?.join(', ') || 'Ingress (default)'} />
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
