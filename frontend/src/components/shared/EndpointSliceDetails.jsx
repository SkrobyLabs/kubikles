import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';

export default function EndpointSliceDetails({ endpointSlice, tabContext }) {
    const metadata = endpointSlice?.metadata || {};
    const endpoints = endpointSlice?.endpoints || [];
    const ports = endpointSlice?.ports || [];

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Address Type', value: endpointSlice?.addressType || '-' },
        { label: 'Endpoints', value: endpoints.length.toString() },
        { label: 'Ports', value: ports.length.toString() },
    ];

    const getServiceName = () => {
        return metadata.labels?.['kubernetes.io/service-name'] || '-';
    };

    const formatPorts = () => {
        if (!ports || ports.length === 0) return '-';
        return ports.map(p => `${p.port}/${p.protocol || 'TCP'}${p.name ? ` (${p.name})` : ''}`).join(', ');
    };

    const getEndpointStatus = (endpoint) => {
        const conditions = endpoint.conditions || {};
        if (conditions.ready === true) return { text: 'Ready', color: 'text-green-400' };
        if (conditions.ready === false) return { text: 'Not Ready', color: 'text-yellow-400' };
        if (conditions.terminating === true) return { text: 'Terminating', color: 'text-red-400' };
        return { text: 'Unknown', color: 'text-gray-400' };
    };

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="EndpointSlice"
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
                        <div>
                            <dt className="text-xs text-gray-500">Service</dt>
                            <dd className="text-sm text-gray-200 mt-0.5">{getServiceName()}</dd>
                        </div>
                    </div>
                </div>

                {/* Ports */}
                {ports.length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Ports</h3>
                        <div className="bg-gray-800/50 rounded-lg p-3">
                            <div className="grid grid-cols-3 gap-2 text-xs text-gray-500 mb-2">
                                <span>Name</span>
                                <span>Port</span>
                                <span>Protocol</span>
                            </div>
                            {ports.map((port, idx) => (
                                <div key={idx} className="grid grid-cols-3 gap-2 text-sm text-gray-200 py-1">
                                    <span className="font-mono">{port.name || '-'}</span>
                                    <span className="font-mono">{port.port}</span>
                                    <span>{port.protocol || 'TCP'}</span>
                                </div>
                            ))}
                        </div>
                    </div>
                )}

                {/* Endpoints */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">
                        Endpoints ({endpoints.length})
                    </h3>
                    {endpoints.length === 0 ? (
                        <p className="text-sm text-gray-500">No endpoints</p>
                    ) : (
                        <div className="space-y-2">
                            {endpoints.map((endpoint, idx) => {
                                const status = getEndpointStatus(endpoint);
                                return (
                                    <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                        <div className="flex items-center justify-between mb-2">
                                            <div className="flex items-center gap-2">
                                                <span className={`text-xs font-medium ${status.color}`}>
                                                    {status.text}
                                                </span>
                                                {endpoint.nodeName && (
                                                    <span className="text-xs text-gray-500">
                                                        Node: {endpoint.nodeName}
                                                    </span>
                                                )}
                                            </div>
                                            {endpoint.zone && (
                                                <span className="text-xs text-gray-500">
                                                    Zone: {endpoint.zone}
                                                </span>
                                            )}
                                        </div>
                                        <div className="space-y-1">
                                            {(endpoint.addresses || []).map((addr, addrIdx) => (
                                                <div key={addrIdx} className="text-sm text-gray-200 font-mono">
                                                    {addr}
                                                </div>
                                            ))}
                                        </div>
                                        {endpoint.targetRef && (
                                            <div className="text-xs text-gray-500 mt-2">
                                                {endpoint.targetRef.kind}: {endpoint.targetRef.name}
                                            </div>
                                        )}
                                        {endpoint.hints?.forZones && endpoint.hints.forZones.length > 0 && (
                                            <div className="text-xs text-gray-500 mt-1">
                                                Hint Zones: {endpoint.hints.forZones.map(z => z.name).join(', ')}
                                            </div>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>

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
        </DetailsPanel>
    );
}
