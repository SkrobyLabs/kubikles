import React from 'react';
import { PencilSquareIcon, ShareIcon, ChartBarIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
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
                {/* Metrics */}
                <DetailSection title="Metrics">
                    {metrics.length === 0 ? (
                        <span className="text-gray-500">No metrics configured</span>
                    ) : (
                        <div className="space-y-2">
                            {metrics.map((metric: any, idx: number) => {
                                const current = currentMetrics[idx];
                                return (
                                    <div key={idx} className="bg-background-dark rounded border border-border p-3 flex justify-between items-center">
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
                </DetailSection>

                {/* Conditions */}
                {conditions.length > 0 && (
                    <DetailSection title="Conditions">
                        <div className="space-y-2">
                            {conditions.map((condition: any, idx: number) => (
                                <div key={idx} className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-0">
                                    <div className="flex items-center gap-2">
                                        <StatusBadge status={condition.type} variant={condition.status === 'True' ? 'success' : condition.status === 'False' ? 'error' : 'warning'} />
                                        <span className="text-sm text-gray-400">{condition.message}</span>
                                    </div>
                                    <span className="text-xs text-gray-500" title={condition.lastTransitionTime}>
                                        {formatAge(condition.lastTransitionTime)}
                                    </span>
                                </div>
                            ))}
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Scale Target" value={getScaleTargetRef()} />
                    <DetailRow label="Min Replicas" value={spec.minReplicas ?? 1} />
                    <DetailRow label="Max Replicas" value={spec.maxReplicas} />
                    <DetailRow label="Current Replicas" value={status.currentReplicas ?? '-'} />
                    <DetailRow label="Desired Replicas" value={status.desiredReplicas ?? '-'} />
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
