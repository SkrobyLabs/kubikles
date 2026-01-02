import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';

export default function CSINodeDetails({ csiNode, tabContext }) {
    const metadata = csiNode?.metadata || {};
    const spec = csiNode?.spec || {};
    const drivers = spec.drivers || [];

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Driver Count', value: drivers.length },
    ];

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="CSI Node"
        >
            <div className="space-y-6 p-4">
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
                            {drivers.map((driver, idx) => (
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
                                                {driver.topologyKeys.map((key, keyIdx) => (
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
                            {metadata.ownerReferences.map((ref, idx) => (
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

                {/* Annotations */}
                {metadata.annotations && Object.keys(metadata.annotations).length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Annotations</h3>
                        <div className="space-y-1">
                            {Object.entries(metadata.annotations).map(([key, value]) => (
                                <div key={key} className="text-xs">
                                    <span className="text-gray-500">{key}:</span>
                                    <span className="ml-1 text-gray-300 break-all">{value}</span>
                                </div>
                            ))}
                        </div>
                    </div>
                )}
            </div>
        </DetailsPanel>
    );
}
