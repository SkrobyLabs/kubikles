import React, { useMemo, useState, useEffect, useCallback } from 'react';
import { PencilSquareIcon, DocumentTextIcon, ShareIcon, CubeIcon, ArrowPathIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useNotification } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, StatusBadge, CopyableLabel, WorkloadImagesRow, getUniqueContainerImages } from './DetailComponents';
import { entriesFromObject, matchesSearch, normalizeSearchTerm, NoSectionMatches, useSectionSearch } from './detailSearch';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';
import ControllerMetricsTab from './ControllerMetricsTab';
import ResourceEventsTab from './ResourceEventsTab';
import ScaleModal from './ScaleModal';
import { ScaleDeployment, RestartDeployment, GetDeploymentYaml } from '~/lib/wailsjs-adapter/go/main/App';
import { useResourceWatcher } from '~/hooks/useResourceWatcher';
import { useCompletionPolling } from '~/hooks/useCompletionPolling';
// @ts-ignore - no declaration file for js-yaml
import yaml from 'js-yaml';

const TAB_BASIC = 'basic';
const TAB_EVENTS = 'events';
const TAB_METRICS = 'metrics';

export default function DeploymentDetails({ deployment: initialDeployment, tabContext = '' }: { deployment: any; tabContext?: string }) {
    const { currentContext, connectionMode } = useK8s();
    const { openTab, closeTab, navigateWithSearch, getDetailTab, setDetailTab } = useUI();
    const { addNotification } = useNotification();
    const activeTab = getDetailTab('deployment', TAB_BASIC);
    const setActiveTab = (tab: string) => setDetailTab('deployment', tab);
    const [showScaleModal, setShowScaleModal] = useState(false);
    const [optimisticReplicas, setOptimisticReplicas] = useState<number | null>(null);

    // Track the current deployment state, updating from watcher
    const [deployment, setDeployment] = useState(initialDeployment);

    const isStale = tabContext && tabContext !== currentContext;
    const resourceContext = tabContext || currentContext;

    const name = deployment.metadata?.name;
    const namespace = deployment.metadata?.namespace;
    const uid = deployment.metadata?.uid;
    const labels = deployment.metadata?.labels || {};
    const annotations = deployment.metadata?.annotations || {};
    const { sectionSearch, getSectionTerm, renderSearch } = useSectionSearch();

    // Subscribe to deployment updates for this specific resource
    const handleWatcherEvent = useCallback((event: any) => {
        // Only update if this event is for our specific deployment
        if (event.resource?.metadata?.uid === uid) {
            if (event.type === 'MODIFIED' || event.type === 'ADDED') {
                setDeployment(event.resource);
            }
        }
    }, [uid]);

    useResourceWatcher(
        'deployments',
        namespace || '',
        handleWatcherEvent,
        Boolean(namespace && !isStale)
    );
    useCompletionPolling(connectionMode === 'polling' && Boolean(namespace && name && !isStale), async (isCurrent) => {
        const latest = yaml.load(await GetDeploymentYaml(namespace, name));
        if (latest && isCurrent()) setDeployment(latest);
    }, [namespace, name, resourceContext]);
    const spec = deployment.spec || {};
    const status = deployment.status || {};

    const actualReplicas = spec.replicas ?? 0;
    const replicas = optimisticReplicas !== null ? optimisticReplicas : actualReplicas;

    // Clear optimistic update when watcher updates the actual value
    useEffect(() => {
        if (optimisticReplicas !== null && actualReplicas === optimisticReplicas) {
            setOptimisticReplicas(null);
        }
    }, [actualReplicas, optimisticReplicas]);
    const readyReplicas = status.readyReplicas ?? 0;
    const availableReplicas = status.availableReplicas ?? 0;
    const updatedReplicas = status.updatedReplicas ?? 0;

    const conditions = status.conditions || [];
    const selector = spec.selector?.matchLabels || {};
    const strategy = spec.strategy?.type || 'RollingUpdate';
    const imageStrings = getUniqueContainerImages(spec.template?.spec);
    const detailRows: any[] = [
        { label: 'Name', value: name },
        { label: 'Namespace', value: namespace },
        { label: 'Strategy', value: strategy },
        {
            label: 'Created',
            value: `${formatAge(deployment.metadata?.creationTimestamp)} ago`,
            title: deployment.metadata?.creationTimestamp
        },
        {
            label: 'UID',
            value: deployment.metadata?.uid?.substring(0, 8) + '...',
            copyValue: deployment.metadata?.uid
        },
    ];
    const labelEntries = useMemo(() => entriesFromObject(labels), [labels]);
    const annotationEntries = useMemo(() => entriesFromObject(annotations), [annotations]);
    const selectorEntries = useMemo(() => entriesFromObject(selector), [selector]);
    const filteredConditions = useMemo(() => conditions.filter((condition: any) => matchesSearch([
        condition.type,
        condition.status,
        condition.reason,
        condition.message,
        condition.lastTransitionTime,
    ], getSectionTerm('conditions'))), [conditions, sectionSearch]);
    const filteredDetailRows = useMemo(() => detailRows.filter((row) => matchesSearch([
        row.label,
        row.value,
        row.copyValue,
        row.title,
    ], getSectionTerm('details'))), [detailRows, sectionSearch]);
    const detailsTermMatchesImages = matchesSearch(['Images', ...imageStrings], getSectionTerm('details'));
    const filteredSelector = useMemo(() => selectorEntries.filter((entry) => matchesSearch([
        entry.key,
        entry.value,
        entry.display,
    ], getSectionTerm('selector'))), [selectorEntries, sectionSearch]);
    const filteredLabels = useMemo(() => labelEntries.filter((entry) => matchesSearch([
        entry.key,
        entry.value,
        entry.display,
    ], getSectionTerm('labels'))), [labelEntries, sectionSearch]);
    const filteredAnnotations = useMemo(() => annotationEntries.filter((entry) => matchesSearch([
        entry.key,
        entry.value,
        entry.display,
    ], getSectionTerm('annotations'))), [annotationEntries, sectionSearch]);

    const handleEditYaml = () => {
        const tabId = `${resourceContext}-yaml-deployment-${namespace}/${name}`;
        openTab({
            id: tabId,
            context: resourceContext,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="deployment"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={resourceContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `${resourceContext}-deps-deployment-${namespace}/${name}`;
        openTab({
            id: tabId,
            context: resourceContext,
            title: `${name}`,
            content: (
                <DependencyGraph
                    tabContext={resourceContext}
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

    const handleRestart = async () => {
        try {
            await RestartDeployment(namespace, name);
            addNotification({ type: 'success', message: `Restarted deployment ${name}` });
        } catch (error: any) {
            addNotification({ type: 'error', message: `Failed to restart ${name}: ${error.message || error}` });
        }
    };

    const handleScale = async (newReplicas: number) => {
        try {
            await ScaleDeployment(namespace, name, newReplicas);
            // Optimistically update the UI
            setOptimisticReplicas(newReplicas);
            addNotification({
                type: 'success',
                message: `Scaled ${name} to ${newReplicas} replica${newReplicas !== 1 ? 's' : ''}`
            });
        } catch (error: any) {
            addNotification({
                type: 'error',
                message: `Failed to scale ${name}: ${error instanceof Error ? error.message : 'Unknown error'}`
            });
            throw error;
        }
    };

    const getReplicaStatus = () => {
        if (readyReplicas === replicas && replicas > 0) return 'success';
        if (readyReplicas > 0) return 'warning';
        if (replicas === 0) return 'default';
        return 'error';
    };

    const getConditionVariant = (condition: any) => {
        if (condition.status === 'True') return 'success';
        if (condition.status === 'False') return 'error';
        return 'warning';
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
                    controllerType="deployment"
                    isStale={!!isStale}
                />
            ) : activeTab === TAB_EVENTS ? (
                <ResourceEventsTab
                    kind="Deployment"
                    namespace={namespace}
                    name={name}
                    uid={uid}
                    isStale={!!isStale}
                    matchLabels={selector}
                />
            ) : (
            <div className="h-full overflow-auto p-4">
                {/* Replica Status */}
                <DetailSection title="Replicas">
                    <div className="grid grid-cols-4 gap-4 mb-2">
                        <button
                            onClick={() => setShowScaleModal(true)}
                            className="text-center p-3 bg-background-dark rounded border border-border hover:border-primary/50 hover:bg-primary/10 cursor-pointer transition-colors"
                            title="Click to scale"
                        >
                            <div className="text-2xl font-bold text-gray-200">{replicas}</div>
                            <div className="text-xs text-gray-500">Desired</div>
                        </button>
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
                    <DetailSection title="Conditions" headerAction={renderSearch('conditions', 'Search conditions...')}>
                        <div className="space-y-2">
                            {filteredConditions.map((condition: any, idx: number) => (
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
                            {normalizeSearchTerm(getSectionTerm('conditions')) && filteredConditions.length === 0 && (
                                <NoSectionMatches term={getSectionTerm('conditions')} />
                            )}
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details" headerAction={renderSearch('details', 'Search details...')}>
                    {filteredDetailRows.map((row: any) => (
                        <React.Fragment key={row.label}>
                            <DetailRow label={row.label}>
                                {row.label === 'UID' ? (
                                    <CopyableLabel value={row.value} copyValue={row.copyValue} />
                                ) : row.title ? (
                                    <span title={row.title}>{row.value}</span>
                                ) : (
                                    row.value
                                )}
                            </DetailRow>
                            {row.label === 'Namespace' && detailsTermMatchesImages && (
                                <WorkloadImagesRow podSpec={spec.template?.spec} />
                            )}
                        </React.Fragment>
                    ))}
                    {filteredDetailRows.length === 0 && detailsTermMatchesImages && <WorkloadImagesRow podSpec={spec.template?.spec} />}
                    {normalizeSearchTerm(getSectionTerm('details')) && !detailsTermMatchesImages && filteredDetailRows.length === 0 && (
                        <NoSectionMatches term={getSectionTerm('details')} />
                    )}
                </DetailSection>

                {/* Selector */}
                <DetailSection title="Selector" headerAction={renderSearch('selector', 'Search selector...')}>
                    {selectorEntries.length === 0 ? (
                        <span className="text-gray-500">None</span>
                    ) : filteredSelector.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {filteredSelector.map((entry) => (
                                <CopyableLabel key={entry.key} value={entry.display} />
                            ))}
                        </div>
                    ) : (
                        <NoSectionMatches term={getSectionTerm('selector')} />
                    )}
                </DetailSection>

                {/* Labels */}
                <DetailSection title="Labels" headerAction={renderSearch('labels', 'Search labels...')}>
                    {labelEntries.length === 0 ? (
                        <span className="text-gray-500">None</span>
                    ) : filteredLabels.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {filteredLabels.map((entry) => (
                                <CopyableLabel key={entry.key} value={entry.display} />
                            ))}
                        </div>
                    ) : (
                        <NoSectionMatches term={getSectionTerm('labels')} />
                    )}
                </DetailSection>

                {/* Annotations */}
                <DetailSection title="Annotations" headerAction={renderSearch('annotations', 'Search annotations...')}>
                    {annotationEntries.length === 0 ? (
                        <span className="text-gray-500">None</span>
                    ) : filteredAnnotations.length > 0 ? (
                        <div className="flex flex-wrap gap-1.5">
                            {filteredAnnotations.map((entry) => (
                                <CopyableLabel
                                    key={entry.key}
                                    value={entry.key.length > 40 ? `${entry.key.substring(0, 40)}...` : entry.key}
                                    copyValue={entry.display}
                                    className="bg-purple-500/10 border-purple-500/30"
                                />
                            ))}
                        </div>
                    ) : (
                        <NoSectionMatches term={getSectionTerm('annotations')} />
                    )}
                </DetailSection>
            </div>
            )}

            {/* Scale Modal */}
            {showScaleModal && (
                <ScaleModal
                    resourceType="Deployment"
                    resourceName={name}
                    namespace={namespace}
                    currentReplicas={replicas}
                    selector={selector}
                    onScale={handleScale}
                    onClose={() => setShowScaleModal(false)}
                />
            )}
        </div>
    );
}
