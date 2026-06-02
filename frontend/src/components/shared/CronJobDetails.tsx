import React, { useMemo } from 'react';
import { PencilSquareIcon, PlayIcon, PauseIcon, ShareIcon, BriefcaseIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, StatusBadge, CopyableLabel, WorkloadImagesRow, getUniqueContainerImages } from './DetailComponents';
import { entriesFromObject, matchesSearch, normalizeSearchTerm, NoSectionMatches, useSectionSearch } from './detailSearch';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';
import ResourceEventsTab from './ResourceEventsTab';

const TAB_BASIC = 'basic';
const TAB_EVENTS = 'events';

export default function CronJobDetails({ cronJob, tabContext = '' }: { cronJob: any; tabContext?: string }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch, getDetailTab, setDetailTab } = useUI();
    const activeTab = getDetailTab('cronjob', TAB_BASIC);
    const setActiveTab = (tab: string) => setDetailTab('cronjob', tab);

    const isStale = tabContext && tabContext !== currentContext;

    const name = cronJob.metadata?.name;
    const namespace = cronJob.metadata?.namespace;
    const uid = cronJob.metadata?.uid;
    const labels = cronJob.metadata?.labels || {};
    const annotations = cronJob.metadata?.annotations || {};
    const { sectionSearch, getSectionTerm, renderSearch } = useSectionSearch();
    const spec = cronJob.spec || {};
    const status = cronJob.status || {};

    const schedule = spec.schedule || '';
    const suspend = spec.suspend || false;
    const concurrencyPolicy = spec.concurrencyPolicy || 'Allow';
    const successfulJobsHistoryLimit = spec.successfulJobsHistoryLimit ?? 3;
    const failedJobsHistoryLimit = spec.failedJobsHistoryLimit ?? 1;
    const startingDeadlineSeconds = spec.startingDeadlineSeconds;

    const lastScheduleTime = status.lastScheduleTime;
    const lastSuccessfulTime = status.lastSuccessfulTime;
    const activeJobs = status.active || [];
    const imageStrings = getUniqueContainerImages(spec.jobTemplate?.spec?.template?.spec);
    const detailRows: any[] = [
        { label: 'Name', value: name },
        { label: 'Namespace', value: namespace },
        { label: 'Concurrency Policy', value: concurrencyPolicy },
        { label: 'Successful Jobs History', value: successfulJobsHistoryLimit },
        { label: 'Failed Jobs History', value: failedJobsHistoryLimit },
        ...(startingDeadlineSeconds ? [{ label: 'Starting Deadline', value: `${startingDeadlineSeconds}s` }] : []),
        { label: 'Created', value: `${formatAge(cronJob.metadata?.creationTimestamp)} ago`, title: cronJob.metadata?.creationTimestamp },
        { label: 'UID', value: cronJob.metadata?.uid?.substring(0, 8) + '...', copyValue: cronJob.metadata?.uid },
    ];
    const labelEntries = useMemo(() => entriesFromObject(labels), [labels]);
    const annotationEntries = useMemo(() => entriesFromObject(annotations), [annotations]);
    const filteredActiveJobs = useMemo(() => activeJobs.filter((jobRef: any) => matchesSearch([
        jobRef.name,
        jobRef.uid,
        'Running',
    ], getSectionTerm('activeJobs'))), [activeJobs, sectionSearch]);
    const filteredDetailRows = useMemo(() => detailRows.filter((row: any) => matchesSearch([
        row.label,
        row.value,
        row.copyValue,
        row.title,
    ], getSectionTerm('details'))), [detailRows, sectionSearch]);
    const detailsTermMatchesImages = matchesSearch(['Images', ...imageStrings], getSectionTerm('details'));
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
        const tabId = `yaml-cronjob-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <YamlEditor
                    resourceType="cronjob"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = () => {
        const tabId = `deps-cronjob-${namespace}/${name}`;
        openTab({
            id: tabId,
            title: `${name}`,
            content: (
                <DependencyGraph
                    resourceType="cronjob"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleViewJobs = () => {
        navigateWithSearch('jobs', `owner:"${name}"`);
    };

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_EVENTS, label: 'Events' },
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
                        status={suspend ? 'Suspended' : 'Active'}
                        variant={suspend ? 'warning' : 'success'}
                    />
                    {activeJobs.length > 0 && (
                        <StatusBadge
                            status={`${activeJobs.length} running`}
                            variant="warning"
                        />
                    )}
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
                            onClick={handleViewJobs}
                            className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="View Jobs"
                        >
                            <BriefcaseIcon className="w-4 h-4" />
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
                    kind="CronJob"
                    namespace={namespace}
                    name={name}
                    uid={uid}
                    isStale={!!isStale}
                />
            ) : (
            <div className="h-full overflow-auto p-4">
                {/* Schedule */}
                <DetailSection title="Schedule">
                    <div className="grid grid-cols-2 gap-4 mb-2">
                        <div className="p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-mono font-bold text-gray-200">{schedule}</div>
                            <div className="text-xs text-gray-500">Cron Expression</div>
                        </div>
                        <div className="p-3 bg-background-dark rounded border border-border">
                            <div className={`text-lg font-bold ${suspend ? 'text-yellow-400' : 'text-green-400'}`}>
                                {suspend ? 'Suspended' : 'Active'}
                            </div>
                            <div className="text-xs text-gray-500">Status</div>
                        </div>
                    </div>
                    <div className="grid grid-cols-2 gap-4 mb-2">
                        <div className="p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-bold text-gray-200">
                                {lastScheduleTime ? formatAge(lastScheduleTime) : 'Never'}
                            </div>
                            <div className="text-xs text-gray-500">Last Scheduled</div>
                        </div>
                        <div className="p-3 bg-background-dark rounded border border-border">
                            <div className="text-lg font-bold text-gray-200">
                                {lastSuccessfulTime ? formatAge(lastSuccessfulTime) : 'Never'}
                            </div>
                            <div className="text-xs text-gray-500">Last Successful</div>
                        </div>
                    </div>
                    <button
                        onClick={handleViewJobs}
                        className="text-sm text-primary hover:text-primary/80 hover:underline"
                    >
                        View Jobs →
                    </button>
                </DetailSection>

                {/* Active Jobs */}
                {activeJobs.length > 0 && (
                    <DetailSection title="Active Jobs" headerAction={renderSearch('activeJobs', 'Search jobs...')}>
                        <div className="space-y-1.5">
                            {filteredActiveJobs.map((jobRef: any, idx: number) => (
                                <div key={idx} className="flex items-center gap-2">
                                    <StatusBadge status="Running" variant="warning" />
                                    <button
                                        onClick={() => navigateWithSearch('jobs', `uid:"${jobRef.uid}"`)}
                                        className="text-primary hover:text-primary/80 hover:underline"
                                    >
                                        {jobRef.name}
                                    </button>
                                </div>
                            ))}
                            {normalizeSearchTerm(getSectionTerm('activeJobs')) && filteredActiveJobs.length === 0 && (
                                <NoSectionMatches term={getSectionTerm('activeJobs')} />
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
                                <WorkloadImagesRow podSpec={spec.jobTemplate?.spec?.template?.spec} />
                            )}
                        </React.Fragment>
                    ))}
                    {filteredDetailRows.length === 0 && detailsTermMatchesImages && <WorkloadImagesRow podSpec={spec.jobTemplate?.spec?.template?.spec} />}
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
