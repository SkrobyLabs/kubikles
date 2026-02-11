import React, { useMemo } from 'react';
import { PencilSquareIcon, PlayIcon, PauseIcon, ShareIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
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

    const handleEditYaml = () => {
        const tabId = `yaml-cronjob-${cronJob.metadata.uid}`;
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
        const tabId = `deps-cronjob-${cronJob.metadata.uid}`;
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
                    <DetailSection title="Active Jobs">
                        <div className="space-y-1.5">
                            {activeJobs.map((jobRef: any, idx: number) => (
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
                        </div>
                    </DetailSection>
                )}

                {/* Details */}
                <DetailSection title="Details">
                    <DetailRow label="Name" value={name} />
                    <DetailRow label="Namespace" value={namespace} />
                    <DetailRow label="Concurrency Policy" value={concurrencyPolicy} />
                    <DetailRow label="Successful Jobs History" value={successfulJobsHistoryLimit} />
                    <DetailRow label="Failed Jobs History" value={failedJobsHistoryLimit} />
                    {startingDeadlineSeconds && (
                        <DetailRow label="Starting Deadline" value={`${startingDeadlineSeconds}s`} />
                    )}
                    <DetailRow label="Created">
                        <span title={cronJob.metadata?.creationTimestamp}>
                            {formatAge(cronJob.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={cronJob.metadata?.uid?.substring(0, 8) + '...'} copyValue={cronJob.metadata?.uid} />
                    </DetailRow>
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
