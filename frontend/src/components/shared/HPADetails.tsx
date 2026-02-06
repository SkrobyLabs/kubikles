import React from 'react';
import { PencilSquareIcon, ShareIcon, ChartBarIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { LabelsDisplay, AnnotationsDisplay } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function HPADetails({ hpa, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = hpa?.metadata || {};
    const spec = hpa?.spec || {};
    const status = hpa?.status || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-hpa-${hpa.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ChartBarIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="hpa"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-hpa-${hpa.metadata?.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ChartBarIcon,
            content: (
                <DependencyGraph
                    resourceType="hpa"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const getScaleTargetRef = () => {
        const ref = spec.scaleTargetRef;
        if (!ref) return '-';
        return `${ref.kind}/${ref.name}`;
    };

    const formatMetric = (metric: any) => {
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

    const formatCurrentMetric = (current: any) => {
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
                            disabled={isStale}
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
            <div className="space-y-6">
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
                            {metrics.map((metric: any, idx: number) => {
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
                            {conditions.map((condition: any, idx: number) => (
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
            </div>
        </div>
    );
}
