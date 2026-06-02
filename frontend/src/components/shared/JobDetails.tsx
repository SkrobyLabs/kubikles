import React, { useMemo } from 'react';
import { PencilSquareIcon, DocumentTextIcon, ShareIcon, CubeIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, StatusBadge, CopyableLabel, WorkloadImagesRow, getUniqueContainerImages } from './DetailComponents';
import { entriesFromObject, matchesSearch, normalizeSearchTerm, NoSectionMatches, useSectionSearch } from './detailSearch';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';
import ResourceEventsTab from './ResourceEventsTab';

const TAB_BASIC = 'basic';
const TAB_EVENTS = 'events';

export default function JobDetails({ job, tabContext = '' }: { job: any; tabContext?: string }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch, getDetailTab, setDetailTab } = useUI();
    const activeTab = getDetailTab('job', TAB_BASIC);
    const setActiveTab = (tab: string) => setDetailTab('job', tab);

    const isStale = tabContext && tabContext !== currentContext;

    const name = job.metadata?.name;
    const namespace = job.metadata?.namespace;
    const uid = job.metadata?.uid;
    const labels = job.metadata?.labels || {};
    const annotations = job.metadata?.annotations || {};
    const { sectionSearch, getSectionTerm, renderSearch } = useSectionSearch();
    const spec = job.spec || {};
    const status = job.status || {};
    const ownerReferences = job.metadata?.ownerReferences || [];

    const completions = spec.completions ?? 1;
    const parallelism = spec.parallelism ?? 1;
    const backoffLimit = spec.backoffLimit ?? 6;
    const activeDeadlineSeconds = spec.activeDeadlineSeconds;
    const ttlSecondsAfterFinished = spec.ttlSecondsAfterFinished;

    const active = status.active ?? 0;
    const succeeded = status.succeeded ?? 0;
    const failed = status.failed ?? 0;
    const conditions = status.conditions || [];

    const controller = ownerReferences.find((ref: any) => ref.controller);
    const imageStrings = getUniqueContainerImages(spec.template?.spec);
    const detailRows: any[] = [
        { label: 'Name', value: name },
        { label: 'Namespace', value: namespace },
        { label: 'Parallelism', value: parallelism },
        { label: 'Backoff Limit', value: backoffLimit },
        ...(activeDeadlineSeconds ? [{ label: 'Active Deadline', value: `${activeDeadlineSeconds}s` }] : []),
        ...(ttlSecondsAfterFinished !== undefined ? [{ label: 'TTL After Finished', value: `${ttlSecondsAfterFinished}s` }] : []),
        ...(status.startTime ? [{ label: 'Started', value: `${formatAge(status.startTime)} ago`, title: status.startTime }] : []),
        ...(status.completionTime ? [{ label: 'Completed', value: `${formatAge(status.completionTime)} ago`, title: status.completionTime }] : []),
        { label: 'Created', value: `${formatAge(job.metadata?.creationTimestamp)} ago`, title: job.metadata?.creationTimestamp },
        { label: 'UID', value: job.metadata?.uid?.substring(0, 8) + '...', copyValue: job.metadata?.uid },
    ];
    const labelEntries = useMemo(() => entriesFromObject(labels), [labels]);
    const annotationEntries = useMemo(() => entriesFromObject(annotations), [annotations]);
    const filteredConditions = useMemo(() => conditions.filter((condition: any) => matchesSearch([
        condition.type,
        condition.status,
        condition.reason,
        condition.message,
        condition.lastTransitionTime,
    ], getSectionTerm('conditions'))), [conditions, sectionSearch]);
    const filteredDetailRows = useMemo(() => detailRows.filter((row: any) => matchesSearch([
        row.label,
        row.value,
        row.copyValue,
        row.title,
    ], getSectionTerm('details'))), [detailRows, sectionSearch]);
    const detailsTermMatchesImages = matchesSearch(['Images', ...imageStrings], getSectionTerm('details'));
    const controllerMatches = !controller || matchesSearch([
        controller.kind,
        controller.name,
        controller.uid,
    ], getSectionTerm('controlledBy'));
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
        const tabId = `yaml-job-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="job"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-job-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="job"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleViewPods = () => {
        navigateWithSearch('pods', `job-name:"${name}"`);
    };

    const handleViewController = () => {
        if (controller) {
            const kindToView = {
                'CronJob': 'cronjobs',
            };
            const viewName = (kindToView as Record<string, string>)[controller.kind];
            if (viewName) {
                navigateWithSearch(viewName, `uid:"${controller.uid}"`);
            }
        }
    };

    const getJobStatus = () => {
        const completeCondition = conditions.find((c: any) => c.type === 'Complete' && c.status === 'True');
        const failedCondition = conditions.find((c: any) => c.type === 'Failed' && c.status === 'True');

        if (completeCondition) return { status: 'Complete', variant: 'success' };
        if (failedCondition) return { status: 'Failed', variant: 'error' };
        if (active > 0) return { status: 'Running', variant: 'warning' };
        return { status: 'Pending', variant: 'default' };
    };

    const jobStatus = getJobStatus();

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_EVENTS, label: 'Events' },
    ], []);

    const getConditionVariant = (condition: any) => {
        if (condition.type === 'Complete' && condition.status === 'True') return 'success';
        if (condition.type === 'Failed' && condition.status === 'True') return 'error';
        return 'default';
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {namespace}/{name}
                    </div>
                    <StatusBadge
                        status={jobStatus.status}
                        variant={jobStatus.variant}
                    />
                    <StatusBadge
                        status={`${succeeded}/${completions}`}
                        variant={succeeded >= completions ? 'success' : active > 0 ? 'warning' : 'default'}
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
                    </div>
                </div>
            </div>

            {/* Content Area */}
            {activeTab === TAB_EVENTS ? (
                <ResourceEventsTab
                    kind="Job"
                    namespace={namespace}
                    name={name}
                    uid={uid}
                    isStale={!!isStale}
                />
            ) : (
            <div className="h-full overflow-auto p-4">
                {/* Status */}
                <DetailSection title="Status">
                    <div className="grid grid-cols-4 gap-4 mb-2">
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className="text-2xl font-bold text-gray-200">{completions}</div>
                            <div className="text-xs text-gray-500">Completions</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className={`text-2xl font-bold ${succeeded >= completions ? 'text-green-400' : 'text-gray-200'}`}>
                                {succeeded}
                            </div>
                            <div className="text-xs text-gray-500">Succeeded</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className={`text-2xl font-bold ${active > 0 ? 'text-blue-400' : 'text-gray-200'}`}>
                                {active}
                            </div>
                            <div className="text-xs text-gray-500">Active</div>
                        </div>
                        <div className="text-center p-3 bg-background-dark rounded border border-border">
                            <div className={`text-2xl font-bold ${failed > 0 ? 'text-red-400' : 'text-gray-200'}`}>
                                {failed}
                            </div>
                            <div className="text-xs text-gray-500">Failed</div>
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
                                        <span className="text-sm text-gray-400">{condition.message || condition.reason}</span>
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

                {/* Controller */}
                {controller && (
                    <DetailSection title="Controlled By" headerAction={renderSearch('controlledBy', 'Search owner...')}>
                        {controllerMatches ? (
                            <div className="flex items-center gap-2">
                                <span className="text-gray-400">{controller.kind}:</span>
                                <button
                                    onClick={handleViewController}
                                    className="text-primary hover:text-primary/80 hover:underline"
                                >
                                    {controller.name}
                                </button>
                            </div>
                        ) : (
                            <NoSectionMatches term={getSectionTerm('controlledBy')} />
                        )}
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
        </div>
    );
}
