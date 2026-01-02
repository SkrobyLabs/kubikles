import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';
import { CopyableLabel } from './DetailComponents';

export default function LeaseDetails({ lease, tabContext }) {
    const metadata = lease?.metadata || {};
    const spec = lease?.spec || {};

    const formatDuration = (seconds) => {
        if (!seconds) return '-';
        if (seconds < 60) return `${seconds}s`;
        if (seconds < 3600) return `${Math.floor(seconds / 60)}m ${seconds % 60}s`;
        return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`;
    };

    const formatTimestamp = (timestamp) => {
        if (!timestamp) return '-';
        const date = new Date(timestamp);
        return date.toLocaleString();
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Lease Duration', value: formatDuration(spec.leaseDurationSeconds) },
        { label: 'Lease Transitions', value: spec.leaseTransitions ?? '-' },
    ];

    const timeInfo = [
        { label: 'Acquire Time', value: formatTimestamp(spec.acquireTime) },
        { label: 'Renew Time', value: formatTimestamp(spec.renewTime) },
    ];

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="Lease"
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

                {/* Timing Information */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Timing</h3>
                    <div className="grid grid-cols-2 gap-4">
                        {timeInfo.map(({ label, value }) => (
                            <div key={label}>
                                <dt className="text-xs text-gray-500">{label}</dt>
                                <dd className="text-sm text-gray-200 mt-0.5">{value}</dd>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Leader Election Info */}
                {spec.holderIdentity && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Leader Election</h3>
                        <div className="bg-gray-800/50 rounded-lg p-3">
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
