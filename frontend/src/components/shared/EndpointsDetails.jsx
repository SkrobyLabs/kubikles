import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';

export default function EndpointsDetails({ endpoints, tabContext }) {
    const metadata = endpoints?.metadata || {};
    const subsets = endpoints?.subsets || [];

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
        <DetailsPanel
            title={metadata.name}
            subtitle="Endpoints"
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
