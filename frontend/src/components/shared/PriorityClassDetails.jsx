import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';

export default function PriorityClassDetails({ priorityClass, tabContext }) {
    const metadata = priorityClass?.metadata || {};

    const formatValue = (value) => {
        if (value >= 1000000000) return `${(value / 1000000000).toFixed(1)}B (${value.toLocaleString()})`;
        if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M (${value.toLocaleString()})`;
        if (value >= 1000) return `${(value / 1000).toFixed(1)}K (${value.toLocaleString()})`;
        return value?.toLocaleString() || '0';
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Value', value: formatValue(priorityClass.value) },
        { label: 'Global Default', value: priorityClass.globalDefault ? 'Yes' : 'No' },
        { label: 'Preemption Policy', value: priorityClass.preemptionPolicy || 'PreemptLowerPriority' },
    ];

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="Priority Class"
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

                {/* Description */}
                {priorityClass.description && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Description</h3>
                        <p className="text-sm text-gray-300 bg-gray-800/50 rounded-lg p-3">
                            {priorityClass.description}
                        </p>
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
