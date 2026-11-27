import type { SemanticsMap } from '~/utils/columnSemantics';
import { getController, controllerRefText, templateContainers, firstImage, imageLines } from '../workloadSemantics';

/** "succeeded/completions" as the Completions column shows it. */
export const jobCompletions = (job: any): string => {
    const succeeded = job.status?.succeeded || 0;
    const completions = job.spec?.completions || '?';
    return `${succeeded}/${completions}`;
};

/** Type of the last condition, '-' without conditions. */
export const jobCondition = (job: any): string => {
    const conditions = job.status?.conditions || [];
    if (conditions.length === 0) return '-';
    const lastCondition = conditions[conditions.length - 1];
    return lastCondition.type || '-';
};

/** Run time in milliseconds: start to completion, or to now while the job has not completed. 0 before it started. */
export const jobDurationMs = (job: any): number => {
    const start = job.status?.startTime;
    const end = job.status?.completionTime;
    if (!start) return 0;
    return (end ? new Date(end) : new Date()).getTime() - new Date(start).getTime();
};

/** The Duration column text, null before the job started. Counts to now while it has not completed. */
export const formatJobDuration = (startTime: string | undefined, completionTime: string | undefined): string | null => {
    if (!startTime) return null;
    const endTime = completionTime ? new Date(completionTime) : new Date();
    const duration = Math.floor((endTime.getTime() - new Date(startTime).getTime()) / 1000);
    if (duration < 60) return `${duration}s`;
    if (duration < 3600) return `${Math.floor(duration / 60)}m ${duration % 60}s`;
    return `${Math.floor(duration / 3600)}h ${Math.floor((duration % 3600) / 60)}m`;
};

/**
 * Sort/filter semantics of the Job list columns. The legacy list gives the name, namespace,
 * completions and condition columns no getValue (their sort key is the missing property of the
 * object, undefined for every row), which is kept as it is.
 */
export const jobSemantics = {
    name: {},
    namespace: {},
    completions: {},
    condition: {},
    age: { getValue: (job: any) => job.metadata?.creationTimestamp },
    controlledBy: { getValue: (item: any) => getController(item)?.kind || '' },
    parallelism: { getValue: (job: any) => job.spec?.parallelism ?? 1 },
    backoffLimit: { getValue: (job: any) => job.spec?.backoffLimit ?? 6 },
    activeDeadline: { getValue: (job: any) => job.spec?.activeDeadlineSeconds ?? 0 },
    startTime: { getValue: (job: any) => job.status?.startTime || '' },
    duration: { getValue: jobDurationMs },
    image: { getValue: (job: any) => firstImage(templateContainers(job)) },
    /** Not columns of the list: the view's display-only cells (the legacy renders read the object). */
    completionsText: { getValue: jobCompletions, filterable: false },
    conditionText: { getValue: jobCondition, filterable: false },
    completionTime: { getValue: (job: any) => job.status?.completionTime || '', filterable: false },
    imageList: { getValue: (job: any) => imageLines(templateContainers(job)), filterable: false },
    controllerRef: { getValue: controllerRefText, filterable: false },
} satisfies SemanticsMap;
