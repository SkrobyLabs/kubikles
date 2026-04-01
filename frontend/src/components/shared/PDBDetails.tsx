import React from 'react';
import { PencilSquareIcon, ShareIcon, ShieldExclamationIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function PDBDetails({ pdb, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { openTab, closeTab } = useUI();

    const metadata = pdb?.metadata || {};
    const spec = pdb?.spec || {};
    const status = pdb?.status || {};

    const isStale = tabContext && tabContext !== currentContext;
    const name = metadata.name;
    const namespace = metadata.namespace;

    const handleEditYaml = () => {
        const tabId = `yaml-pdb-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldExclamationIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="pdb"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-pdb-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            icon: ShieldExclamationIcon,
            content: (
                <DependencyGraph
                    resourceType="pdb"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const getSelector = () => {
        const selector = spec.selector;
        if (!selector) return '-';
        const labels = selector.matchLabels || {};
        if (Object.keys(labels).length === 0) return 'All Pods';
        return Object.entries(labels)
            .map(([k, v]) => `${k}=${v}`)
            .join(', ');
    };

    const getBudgetValue = () => {
        if (spec.minAvailable !== undefined) {
            return { type: 'minAvailable', value: spec.minAvailable };
        }
        if (spec.maxUnavailable !== undefined) {
            return { type: 'maxUnavailable', value: spec.maxUnavailable };
        }
        return { type: 'none', value: '-' };
    };

    const budget = getBudgetValue();

    const conditions = status.conditions || [];

    const getHealthStatus = () => {
        const current = status.currentHealthy ?? 0;
        const desired = status.desiredHealthy ?? 0;
        if (current >= desired) {
            return { text: 'Healthy', color: 'text-green-400' };
        }
        return { text: 'Unhealthy', color: 'text-red-400' };
    };

    const healthStatus = getHealthStatus();

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
                {/* Status Summary */}
                <DetailSection title="Status">
                    <div className="grid grid-cols-2 gap-4 mb-2">
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className={`text-2xl font-bold ${healthStatus.color}`}>{healthStatus.text}</div>
                            <div className="text-xs text-gray-500">Health</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{status.disruptionsAllowed ?? 0}</div>
                            <div className="text-xs text-gray-500">Disruptions Allowed</div>
                        </div>
                    </div>
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

                {/* Configuration */}
                <DetailSection title="Configuration">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Selector" value={getSelector()} />
                    <DetailRow label="Budget Type" value={budget.type === 'minAvailable' ? 'Min Available' : 'Max Unavailable'} />
                    <DetailRow label="Budget Value" value={budget.value} />
                    <DetailRow label="Created">
                        <span title={metadata.creationTimestamp}>
                            {formatAge(metadata.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={metadata.uid?.substring(0, 8) + '...'} copyValue={metadata.uid} />
                    </DetailRow>
                </DetailSection>

                {/* Current Status */}
                <DetailSection title="Current Status">
                    <DetailRow label="Current Healthy" value={status.currentHealthy ?? '-'} />
                    <DetailRow label="Desired Healthy" value={status.desiredHealthy ?? '-'} />
                    <DetailRow label="Disruptions Allowed" value={status.disruptionsAllowed ?? '-'} />
                    <DetailRow label="Expected Pods" value={status.expectedPods ?? '-'} />
                    <DetailRow label="Observed Generation" value={status.observedGeneration ?? '-'} />
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
