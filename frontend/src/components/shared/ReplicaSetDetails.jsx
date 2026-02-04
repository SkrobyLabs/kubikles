import React, { useMemo, useState } from 'react';
import { PencilSquareIcon, DocumentTextIcon, ShareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context';
import { useUI } from '../../context';
import { formatAge } from '../../utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';
import ControllerMetricsTab from './ControllerMetricsTab';

const TAB_BASIC = 'basic';
const TAB_METRICS = 'metrics';

export default function ReplicaSetDetails({ replicaSet, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch, getDetailTab, setDetailTab } = useUI();
    const activeTab = getDetailTab('replicaset', TAB_BASIC);
    const setActiveTab = (tab) => setDetailTab('replicaset', tab);

    const isStale = tabContext && tabContext !== currentContext;

    const name = replicaSet.metadata?.name;
    const namespace = replicaSet.metadata?.namespace;
    const labels = replicaSet.metadata?.labels || {};
    const annotations = replicaSet.metadata?.annotations || {};
    const spec = replicaSet.spec || {};
    const status = replicaSet.status || {};
    const ownerReferences = replicaSet.metadata?.ownerReferences || [];

    const replicas = spec.replicas ?? 0;
    const readyReplicas = status.readyReplicas ?? 0;
    const availableReplicas = status.availableReplicas ?? 0;
    const fullyLabeledReplicas = status.fullyLabeledReplicas ?? 0;

    const selector = spec.selector?.matchLabels || {};
    const controller = ownerReferences.find(ref => ref.controller);

    const handleEditYaml = () => {
        const tabId = `yaml-replicaset-${replicaSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="replicaset"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-replicaset-${replicaSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="replicaset"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleViewPods = () => {
        const selectorParts = Object.entries(selector).map(([k, v]) => `${k}=${v}`);
        if (selectorParts.length > 0) {
            navigateWithSearch('pods', `labels:"${selectorParts.join(',')}"`);
        }
    };

    const handleViewController = () => {
        if (controller) {
            const kindToView = {
                'Deployment': 'deployments',
            };
            const viewName = kindToView[controller.kind];
            if (viewName) {
                navigateWithSearch(viewName, `uid:"${controller.uid}"`);
            }
        }
    };

    const getReplicaStatus = () => {
        if (readyReplicas === replicas && replicas > 0) return 'success';
        if (readyReplicas > 0) return 'warning';
        if (replicas === 0) return 'default';
        return 'error';
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
                    controllerType="replicaset"
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
                            <div className="text-2xl font-bold text-gray-200">{availableReplicas}</div>
                            <div className="text-xs text-gray-500">Available</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{fullyLabeledReplicas}</div>
                            <div className="text-xs text-gray-500">Labeled</div>
                        </div>
                    </div>
                    <button
                        onClick={handleViewPods}
                        className="text-sm text-primary hover:text-primary/80 hover:underline"
                    >
                        View Pods →
                    </button>
                </DetailSection>

                {/* Controller */}
                {controller && (
                    <DetailSection title="Controlled By">
                        <div className="flex items-center gap-2">
                            <span className="text-gray-400">{controller.kind}:</span>
                            <button
                                onClick={handleViewController}
                                className="text-primary hover:text-primary/80 hover:underline"
                            >
                                {controller.name}
                            </button>
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Created">
                        <span title={replicaSet.metadata?.creationTimestamp}>
                            {formatAge(replicaSet.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={replicaSet.metadata?.uid?.substring(0, 8) + '...'} copyValue={replicaSet.metadata?.uid} />
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
