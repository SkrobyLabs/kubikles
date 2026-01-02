import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';

export default function LimitRangeDetails({ limitRange, tabContext }) {
    const metadata = limitRange?.metadata || {};
    const spec = limitRange?.spec || {};

    const limits = spec.limits || [];

    const formatResource = (value) => {
        if (!value) return '-';
        if (typeof value === 'object') {
            return Object.entries(value)
                .map(([k, v]) => `${k}: ${v}`)
                .join(', ');
        }
        return String(value);
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Limit Types', value: limits.length },
    ];

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="Limit Range"
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

                {/* Limits */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Limits</h3>
                    {limits.length === 0 ? (
                        <p className="text-sm text-gray-500">No limits defined</p>
                    ) : (
                        <div className="space-y-4">
                            {limits.map((limit, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-4">
                                    <div className="flex items-center gap-2 mb-3">
                                        <span className="text-sm font-medium text-gray-200">{limit.type}</span>
                                        <span className="text-xs text-gray-500 bg-gray-700 px-2 py-0.5 rounded">
                                            Type
                                        </span>
                                    </div>
                                    <div className="overflow-x-auto">
                                        <table className="w-full text-sm">
                                            <thead>
                                                <tr className="text-left text-xs text-gray-500 border-b border-gray-700">
                                                    <th className="pb-2 pr-4">Resource</th>
                                                    <th className="pb-2 pr-4">Min</th>
                                                    <th className="pb-2 pr-4">Max</th>
                                                    <th className="pb-2 pr-4">Default</th>
                                                    <th className="pb-2 pr-4">Default Request</th>
                                                    <th className="pb-2">Max Limit/Request Ratio</th>
                                                </tr>
                                            </thead>
                                            <tbody>
                                                {['cpu', 'memory', 'storage', 'ephemeral-storage'].map((resource) => {
                                                    const min = limit.min?.[resource];
                                                    const max = limit.max?.[resource];
                                                    const defaultVal = limit.default?.[resource];
                                                    const defaultRequest = limit.defaultRequest?.[resource];
                                                    const ratio = limit.maxLimitRequestRatio?.[resource];

                                                    if (!min && !max && !defaultVal && !defaultRequest && !ratio) {
                                                        return null;
                                                    }

                                                    return (
                                                        <tr key={resource} className="border-b border-gray-700/50">
                                                            <td className="py-2 pr-4 text-gray-300">{resource}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{min || '-'}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{max || '-'}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{defaultVal || '-'}</td>
                                                            <td className="py-2 pr-4 text-gray-400">{defaultRequest || '-'}</td>
                                                            <td className="py-2 text-gray-400">{ratio || '-'}</td>
                                                        </tr>
                                                    );
                                                })}
                                            </tbody>
                                        </table>
                                    </div>
                                </div>
                            ))}
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
