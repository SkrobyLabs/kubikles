import React from 'react';
import DetailsPanel from './DetailsPanel';
import { formatAge } from '../../utils/formatting';

export default function HPADetails({ hpa, tabContext }) {
    const metadata = hpa?.metadata || {};
    const spec = hpa?.spec || {};
    const status = hpa?.status || {};

    const getScaleTargetRef = () => {
        const ref = spec.scaleTargetRef;
        if (!ref) return '-';
        return `${ref.kind}/${ref.name}`;
    };

    const formatMetric = (metric) => {
        if (!metric) return '-';
        const type = metric.type;
        switch (type) {
            case 'Resource': {
                const resource = metric.resource;
                const target = resource?.target;
                let targetStr = '-';
                if (target?.type === 'Utilization') {
                    targetStr = `${target.averageUtilization}%`;
                } else if (target?.type === 'AverageValue') {
                    targetStr = target.averageValue;
                } else if (target?.type === 'Value') {
                    targetStr = target.value;
                }
                return `${resource?.name} (${targetStr})`;
            }
            case 'Pods':
                return `Pods: ${metric.pods?.metric?.name || '-'}`;
            case 'Object':
                return `Object: ${metric.object?.metric?.name || '-'}`;
            case 'External':
                return `External: ${metric.external?.metric?.name || '-'}`;
            default:
                return type;
        }
    };

    const formatCurrentMetric = (current) => {
        if (!current) return '-';
        const type = current.type;
        switch (type) {
            case 'Resource': {
                const resource = current.resource;
                if (resource?.current?.averageUtilization !== undefined) {
                    return `${resource.current.averageUtilization}%`;
                }
                return resource?.current?.averageValue || '-';
            }
            case 'Pods':
                return current.pods?.current?.averageValue || '-';
            case 'Object':
                return current.object?.current?.value || '-';
            case 'External':
                return current.external?.current?.value || '-';
            default:
                return '-';
        }
    };

    const basicInfo = [
        { label: 'Name', value: metadata.name },
        { label: 'Namespace', value: metadata.namespace },
        { label: 'Age', value: formatAge(metadata.creationTimestamp) },
        { label: 'Scale Target', value: getScaleTargetRef() },
        { label: 'Min Replicas', value: spec.minReplicas ?? 1 },
        { label: 'Max Replicas', value: spec.maxReplicas },
        { label: 'Current Replicas', value: status.currentReplicas ?? '-' },
        { label: 'Desired Replicas', value: status.desiredReplicas ?? '-' },
    ];

    const metrics = spec.metrics || [];
    const currentMetrics = status.currentMetrics || [];

    const conditions = status.conditions || [];

    return (
        <DetailsPanel
            title={metadata.name}
            subtitle="Horizontal Pod Autoscaler"
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

                {/* Metrics */}
                <div>
                    <h3 className="text-sm font-medium text-gray-400 mb-3">Metrics</h3>
                    {metrics.length === 0 ? (
                        <p className="text-sm text-gray-500">No metrics configured</p>
                    ) : (
                        <div className="space-y-2">
                            {metrics.map((metric, idx) => {
                                const current = currentMetrics[idx];
                                return (
                                    <div key={idx} className="bg-gray-800/50 rounded-lg p-3 flex justify-between items-center">
                                        <div>
                                            <div className="text-sm text-gray-300">{formatMetric(metric)}</div>
                                            <div className="text-xs text-gray-500">Target</div>
                                        </div>
                                        <div className="text-right">
                                            <div className="text-sm text-gray-300">{formatCurrentMetric(current)}</div>
                                            <div className="text-xs text-gray-500">Current</div>
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>

                {/* Conditions */}
                {conditions.length > 0 && (
                    <div>
                        <h3 className="text-sm font-medium text-gray-400 mb-3">Conditions</h3>
                        <div className="space-y-2">
                            {conditions.map((condition, idx) => (
                                <div key={idx} className="bg-gray-800/50 rounded-lg p-3">
                                    <div className="flex justify-between items-start">
                                        <div>
                                            <span className={`text-sm ${condition.status === 'True' ? 'text-green-400' : 'text-gray-300'}`}>
                                                {condition.type}
                                            </span>
                                            <span className={`ml-2 text-xs ${condition.status === 'True' ? 'text-green-500' : 'text-gray-500'}`}>
                                                ({condition.status})
                                            </span>
                                        </div>
                                    </div>
                                    {condition.message && (
                                        <p className="text-xs text-gray-500 mt-1">{condition.message}</p>
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
