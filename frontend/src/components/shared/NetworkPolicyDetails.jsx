import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';

export default function NetworkPolicyDetails({ networkPolicy, tabContext }) {
    const metadata = networkPolicy?.metadata || {};
    const spec = networkPolicy?.spec || {};

    const formatLabelSelector = (selector) => {
        if (!selector || Object.keys(selector.matchLabels || {}).length === 0) {
            return 'All Pods';
        }
        return Object.entries(selector.matchLabels || {})
            .map(([k, v]) => `${k}=${v}`)
            .join(', ');
    };

    const formatPort = (port) => {
        if (!port) return 'All';
        const protocol = port.protocol || 'TCP';
        const portNum = port.port || 'All';
        return `${portNum}/${protocol}`;
    };

    const formatPeer = (peer) => {
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

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Pod Selector', value: formatLabelSelector(spec.podSelector) },
        { label: 'Policy Types', value: spec.policyTypes?.join(', ') || 'Ingress (default)' },
    ];

    const ingressRules = spec.ingress || [];
    const egressRules = spec.egress || [];

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="Network Policy"
        >
            <div className="space-y-6 p-4">
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

                {/* Ingress Rules */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">
                        Ingress Rules ({ingressRules.length})
                    </h3>
                    {ingressRules.length === 0 ? (
                        <p className="text-sm text-gray-500">
                            {spec.policyTypes?.includes('Ingress')
                                ? 'No ingress rules (all ingress denied)'
                                : 'Ingress not restricted'}
                        </p>
                    ) : (
                        <div className="space-y-3">
                            {ingressRules.map((rule, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="text-xs text-gray-400 mb-2">Rule {idx + 1}</div>
                                    <div className="space-y-2">
                                        <div>
                                            <span className="text-xs text-gray-500">From:</span>
                                            <div className="text-sm text-gray-300 ml-2">
                                                {rule.from?.length > 0
                                                    ? rule.from.map((peer, i) => (
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
                </div>

                {/* Egress Rules */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">
                        Egress Rules ({egressRules.length})
                    </h3>
                    {egressRules.length === 0 ? (
                        <p className="text-sm text-gray-500">
                            {spec.policyTypes?.includes('Egress')
                                ? 'No egress rules (all egress denied)'
                                : 'Egress not restricted'}
                        </p>
                    ) : (
                        <div className="space-y-3">
                            {egressRules.map((rule, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="text-xs text-gray-400 mb-2">Rule {idx + 1}</div>
                                    <div className="space-y-2">
                                        <div>
                                            <span className="text-xs text-gray-500">To:</span>
                                            <div className="text-sm text-gray-300 ml-2">
                                                {rule.to?.length > 0
                                                    ? rule.to.map((peer, i) => (
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
                </div>

                {/* Labels */}
                {metadata.labels && Object.keys(metadata.labels).length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Labels</h3>
                        <div className="flex flex-wrap gap-2">
                            {Object.entries(metadata.labels).map(([key, value]) => (
                                <span
                                    key={key}
                                    className="inline-flex items-center px-2 py-1 rounded text-xs bg-gray-700 text-gray-300"
                                >
                                    {key}: {value}
                                </span>
                            ))}
                        </div>
                    </div>
                )}
            </div>
        </DetailsPanel>
    );
}
