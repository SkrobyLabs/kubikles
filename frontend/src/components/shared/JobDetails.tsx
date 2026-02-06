import React from 'react';
import { PencilSquareIcon, DocumentTextIcon, ShareIcon, CubeIcon } from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { formatAge } from '~/utils/formatting';
import { DetailRow, DetailSection, LabelsDisplay, AnnotationsDisplay, StatusBadge, CopyableLabel } from './DetailComponents';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../lazy';

export default function JobDetails({ job, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openTab, closeTab, navigateWithSearch } = useUI();

    const isStale = tabContext && tabContext !== currentContext;

    const name = job.metadata?.name;
    const namespace = job.metadata?.namespace;
    const labels = job.metadata?.labels || {};
    const annotations = job.metadata?.annotations || {};
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

    const controller = ownerReferences.find(ref => ref.controller);

    const handleEditYaml = () => {
        const tabId = `yaml-job-${job.metadata.uid}`;
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
        const tabId = `deps-job-${job.metadata.uid}`;
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
            const viewName = kindToView[controller.kind];
            if (viewName) {
                navigateWithSearch(viewName, `uid:"${controller.uid}"`);
            }
        }
    };

    const getJobStatus = () => {
        const completeCondition = conditions.find(c => c.type === 'Complete' && c.status === 'True');
        const failedCondition = conditions.find(c => c.type === 'Failed' && c.status === 'True');

        if (completeCondition) return { status: 'Complete', variant: 'success' };
        if (failedCondition) return { status: 'Failed', variant: 'error' };
        if (active > 0) return { status: 'Running', variant: 'warning' };
        return { status: 'Pending', variant: 'default' };
    };

    const jobStatus = getJobStatus();

    const getConditionVariant = (condition) => {
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
                    <DetailSection title="Conditions">
                        <div className="space-y-2">
                            {conditions.map((condition, idx) => (
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
                        </div>
                    </DetailSection>
                )}

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
                    <DetailRow label="Parallelism" value={parallelism} />
                    <DetailRow label="Backoff Limit" value={backoffLimit} />
                    {activeDeadlineSeconds && (
                        <DetailRow label="Active Deadline" value={`${activeDeadlineSeconds}s`} />
                    )}
                    {ttlSecondsAfterFinished !== undefined && (
                        <DetailRow label="TTL After Finished" value={`${ttlSecondsAfterFinished}s`} />
                    )}
                    {status.startTime && (
                        <DetailRow label="Started">
                            <span title={status.startTime}>
                                {formatAge(status.startTime)} ago
                            </span>
                        </DetailRow>
                    )}
                    {status.completionTime && (
                        <DetailRow label="Completed">
                            <span title={status.completionTime}>
                                {formatAge(status.completionTime)} ago
                            </span>
                        </DetailRow>
                    )}
                    <DetailRow label="Created">
                        <span title={job.metadata?.creationTimestamp}>
                            {formatAge(job.metadata?.creationTimestamp)} ago
                        </span>
                    </DetailRow>
                    <DetailRow label="UID">
                        <CopyableLabel value={job.metadata?.uid?.substring(0, 8) + '...'} copyValue={job.metadata?.uid} />
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
        </div>
    );
}
