import React, { useMemo, useState } from 'react';
import { PencilSquareIcon, DocumentTextIcon, ShareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import { formatAge } from '../../utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import YamlEditor from './YamlEditor';
import DependencyGraph from './DependencyGraph';
import ControllerMetricsTab from './ControllerMetricsTab';

const TAB_BASIC = 'basic';
const TAB_METRICS = 'metrics';

export default function DeploymentDetails({ deployment, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch } = useUI();
    const [activeTab, setActiveTab] = useState(TAB_BASIC);

    const isStale = tabContext && tabContext !== currentContext;

    const name = deployment.metadata?.name;
    const namespace = deployment.metadata?.namespace;
    const labels = deployment.metadata?.labels || {};
    const annotations = deployment.metadata?.annotations || {};
    const spec = deployment.spec || {};
    const status = deployment.status || {};

    const replicas = spec.replicas ?? 0;
    const readyReplicas = status.readyReplicas ?? 0;
    const availableReplicas = status.availableReplicas ?? 0;
    const updatedReplicas = status.updatedReplicas ?? 0;

    const conditions = status.conditions || [];
    const selector = spec.selector?.matchLabels || {};
    const strategy = spec.strategy?.type || 'RollingUpdate';

    const handleEditYaml = () => {
        const tabId = `yaml-deployment-${deployment.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${name}`,
            content: (
                <YamlEditor
                    resourceType="deployment"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-deployment-${deployment.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${name}`,
            content: (
                <DependencyGraph
                    resourceType="deployment"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleViewPods = () => {
        // Build selector query from matchLabels
        const selectorParts = Object.entries(selector).map(([k, v]) => `${k}=${v}`);
        if (selectorParts.length > 0) {
            navigateWithSearch('pods', `labels:"${selectorParts.join(',')}"`);
        }
    };

    const getReplicaStatus = () => {
        if (readyReplicas === replicas && replicas > 0) return 'success';
        if (readyReplicas > 0) return 'warning';
        if (replicas === 0) return 'default';
        return 'error';
    };

    const getConditionVariant = (condition) => {
        if (condition.status === 'True') return 'success';
        if (condition.status === 'False') return 'error';
        return 'warning';
    };

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_METRICS, label: 'Metrics' },
    ], []);

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {namespace}/{name}
                    </div>
                    <StatusBadge
                        status={`${readyReplicas}/${replicas}`}
                        variant={getReplicaStatus()}
                    />
                    {/* Tab Toggle */}
                    <div className="flex items-center bg-surface-light rounded-md p-0.5">
                        {tabs.map((tab) => (
                            <button
                                key={tab.id}
                                onClick={() => setActiveTab(tab.id)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    activeTab === tab.id
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                {tab.label}
                            </button>
                        ))}
                    </div>
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        <button
                            onClick={handleViewPods}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="View Pods"
                        >
                            <DocumentTextIcon className="w-4 h-4" />
                        </button>
                        <button
                            onClick={handleEditYaml}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
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
            {activeTab === TAB_METRICS ? (
                <ControllerMetricsTab
                    namespace={namespace}
                    name={name}
                    controllerType="deployment"
                    isStale={isStale}
                />
            ) : (
            <div className="h-full overflow-auto p-4">
                {/* Replica Status */}
                <DetailSection title="Replicas">
                    <div className="grid grid-cols-4 gap-4 mb-2">
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{replicas}</div>
                            <div className="text-xs text-gray-500">Desired</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className={`text-2xl font-bold ${readyReplicas === replicas ? 'text-green-400' : 'text-yellow-400'}`}>
                                {readyReplicas}
                            </div>
                            <div className="text-xs text-gray-500">Ready</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{updatedReplicas}</div>
                            <div className="text-xs text-gray-500">Updated</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{availableReplicas}</div>
                            <div className="text-xs text-gray-500">Available</div>
                        </div>
                    </div>
                    <button
                        onClick={handleViewPods}
                        className="text-sm text-primary hover:text-primary/80 hover:underline"
                    >
                        View Pods →
                    </button>
                </DetailSection>

                {/* Conditions */}
                {conditions.length > 0 && (
                    <DetailSection title="Conditions">
                        <div className="space-y-2">
                            {conditions.map((condition, idx) => (
                                <div key={idx} className="flex items-center justify-between py-1.5 border-b border-border/50 last:border-0">
                                    <div className="flex items-center gap-2">
                                        <StatusBadge status={condition.type} variant={getConditionVariant(condition)} />
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
                    <DetailRow label="Strategy" value={strategy} />
                    <DetailRow label="Created">
                        <span title={deployment.metadata?.creationTimestamp}>
                            {formatAge(deployment.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={deployment.metadata?.uid?.substring(0, 8) + '...'} copyValue={deployment.metadata?.uid} />
                    </DetailRow>
                </DetailSection>

                {/* Selector */}
                <DetailSection title="Selector">
                    {Object.keys(selector).length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {Object.entries(selector).map(([key, value]) => (
                                <CopyableLabel key={key} value={`${key}=${value}`} />
                            ))}
                        </div>
                    ) : (
                        <span className="text-gray-500">None</span>
                    )}
                </DetailSection>

                {/* Labels */}
                <DetailSection title="Labels">
                    <LabelsDisplay labels={labels} />
                </DetailSection>

                {/* Annotations */}
                <DetailSection title="Annotations">
                    <AnnotationsDisplay annotations={annotations} />
                </DetailSection>
            </div>
            )}
        </div>
    );
}
