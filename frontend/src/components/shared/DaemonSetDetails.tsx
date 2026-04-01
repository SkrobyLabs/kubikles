import React, { useMemo, useState } from 'react';
import { PencilSquareIcon, DocumentTextIcon, ShareIcon, CubeIcon, ArrowPathIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useNotification } from '~/context';
import { RestartDaemonSet } from '~/lib/wailsjs-adapter/go/main/App';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';
import ControllerMetricsTab from './ControllerMetricsTab';
import ResourceEventsTab from './ResourceEventsTab';

const TAB_BASIC = 'basic';
const TAB_EVENTS = 'events';
const TAB_METRICS = 'metrics';

export default function DaemonSetDetails({ daemonSet, tabContext = '' }: { daemonSet: any; tabContext?: string }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch, getDetailTab, setDetailTab } = useUI();
    const { addNotification } = useNotification();
    const activeTab = getDetailTab('daemonset', TAB_BASIC);
    const setActiveTab = (tab: string) => setDetailTab('daemonset', tab);

    const isStale = tabContext && tabContext !== currentContext;

    const name = daemonSet.metadata?.name;
    const namespace = daemonSet.metadata?.namespace;
    const uid = daemonSet.metadata?.uid;
    const labels = daemonSet.metadata?.labels || {};
    const annotations = daemonSet.metadata?.annotations || {};
    const spec = daemonSet.spec || {};
    const status = daemonSet.status || {};

    const desiredNumberScheduled = status.desiredNumberScheduled ?? 0;
    const currentNumberScheduled = status.currentNumberScheduled ?? 0;
    const numberReady = status.numberReady ?? 0;
    const numberAvailable = status.numberAvailable ?? 0;
    const numberMisscheduled = status.numberMisscheduled ?? 0;
    const updatedNumberScheduled = status.updatedNumberScheduled ?? 0;

    const selector = spec.selector?.matchLabels || {};
    const updateStrategy = spec.updateStrategy?.type || 'RollingUpdate';

    const handleEditYaml = () => {
        const tabId = `yaml-daemonset-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="daemonset"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-daemonset-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="daemonset"
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

    const handleRestart = async () => {
        try {
            await RestartDaemonSet(namespace, name);
            addNotification({ type: 'success', message: `Restarted daemonset ${name}` });
        } catch (error: any) {
            addNotification({ type: 'error', message: `Failed to restart ${name}: ${error.message || error}` });
        }
    };

    const getStatus = () => {
        if (numberReady === desiredNumberScheduled && desiredNumberScheduled > 0) return 'success';
        if (numberReady > 0) return 'warning';
        if (desiredNumberScheduled === 0) return 'default';
        return 'error';
    };

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_EVENTS, label: 'Events' },
        { id: TAB_METRICS, label: 'Metrics' },
    ], []);

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {namespace}/{name}
                    </div>
                    <StatusBadge
                        status={`${numberReady}/${desiredNumberScheduled}`}
                        variant={getStatus()}
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
                            <CubeIcon className="w-4 h-4" />
                        </button>
                        <button
                            onClick={handleEditYaml}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Edit YAML"
                            disabled={!!isStale}
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
                        <button
                            onClick={handleRestart}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Restart"
                            disabled={!!isStale}
                        >
                            <ArrowPathIcon className="w-4 h-4" />
                        </button>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            {activeTab === TAB_METRICS ? (
                <ControllerMetricsTab
                    namespace={namespace}
                    name={name}
                    controllerType="daemonset"
                    isStale={!!isStale}
                />
            ) : activeTab === TAB_EVENTS ? (
                <ResourceEventsTab
                    kind="DaemonSet"
                    namespace={namespace}
                    name={name}
                    uid={uid}
                    isStale={!!isStale}
                    matchLabels={selector}
                />
            ) : (
            <div className="h-full overflow-auto p-4">
                {/* Status */}
                <DetailSection title="Status">
                    <div className="grid grid-cols-3 gap-4 mb-2">
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{desiredNumberScheduled}</div>
                            <div className="text-xs text-gray-500">Desired</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{currentNumberScheduled}</div>
                            <div className="text-xs text-gray-500">Current</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className={`text-2xl font-bold ${numberReady === desiredNumberScheduled ? 'text-green-400' : 'text-yellow-400'}`}>
                                {numberReady}
                            </div>
                            <div className="text-xs text-gray-500">Ready</div>
                        </div>
                    </div>
                    <div className="grid grid-cols-3 gap-4 mb-2">
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{numberAvailable}</div>
                            <div className="text-xs text-gray-500">Available</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{updatedNumberScheduled}</div>
                            <div className="text-xs text-gray-500">Updated</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className={`text-2xl font-bold ${numberMisscheduled > 0 ? 'text-red-400' : 'text-gray-200'}`}>
                                {numberMisscheduled}
                            </div>
                            <div className="text-xs text-gray-500">Misscheduled</div>
                        </div>
                    </div>
                    <button
                        onClick={handleViewPods}
                        className="text-sm text-primary hover:text-primary/80 hover:underline"
                    >
                        View Pods →
                    </button>
                </DetailSection>

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Update Strategy" value={updateStrategy} />
                    <DetailRow label="Created">
                        <span title={daemonSet.metadata?.creationTimestamp}>
                            {formatAge(daemonSet.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={daemonSet.metadata?.uid?.substring(0, 8) + '...'} copyValue={daemonSet.metadata?.uid} />
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
