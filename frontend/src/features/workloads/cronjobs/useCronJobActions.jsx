import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteCronJob, TriggerCronJob, SuspendCronJob, ListJobs } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import LogViewer from '../../../components/shared/LogViewer';
import Logger from '../../../utils/Logger';

export const useCronJobActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext, currentNamespace, triggerRefresh } = useK8s();

    const handleViewLogs = async (cronJob) => {
        Logger.info("View logs for CronJob", { namespace: cronJob.metadata.namespace, name: cronJob.metadata.name });
        const namespace = cronJob.metadata.namespace;

        try {
            const allJobs = await ListJobs(namespace);

            // Filter jobs that belong to this cronjob
            const cronJobJobs = allJobs.filter(job => {
                const ownerRefs = job.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'CronJob' &&
                    ref.name === cronJob.metadata.name
                );
            });

            if (cronJobJobs.length === 0) {
                Logger.info("No jobs found for CronJob", { namespace, name: cronJob.metadata.name });
                alert(`No jobs found for cronjob "${cronJob.metadata.name}". The cronjob may not have run yet.`);
                return;
            }

            // Sort by creation time and get most recent job
            cronJobJobs.sort((a, b) =>
                new Date(b.metadata.creationTimestamp) - new Date(a.metadata.creationTimestamp)
            );
            const mostRecentJob = cronJobJobs[0];

            // Get pods for the most recent job
            const { ListPods } = await import('../../../../wailsjs/go/main/App');
            const allPods = await ListPods(namespace);

            const jobPods = allPods.filter(pod =>
                pod.metadata?.labels?.['job-name'] === mostRecentJob.metadata.name
            );

            if (jobPods.length === 0) {
                Logger.info("No pods found for Job", { namespace, job: mostRecentJob.metadata.name });
                alert(`No pods found for job "${mostRecentJob.metadata.name}".`);
                return;
            }

            const pod = jobPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            // Build container map for all pods
            const podContainerMap = {};
            for (const p of jobPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            Logger.info("Opening logs for CronJob pod", {
                namespace,
                cronJob: cronJob.metadata.name,
                job: mostRecentJob.metadata.name,
                pod: pod.metadata.name,
                totalJobs: cronJobJobs.length
            });

            const tabId = `logs-cronjob-${cronJob.metadata.name}`;
            openTab({
                id: tabId,
                title: `Logs: ${cronJob.metadata.name}`,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={jobPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={cronJob.metadata.name}
                    />
                )
            });
        } catch (err) {
            Logger.error("Failed to get logs for CronJob", err);
            alert(`Failed to get logs for cronjob: ${err.message || err}`);
        }
    };

    const handleEditYaml = (cronJob) => {
        Logger.info("Opening YAML editor for CronJob", { namespace: cronJob.metadata.namespace, name: cronJob.metadata.name });
        const tabId = `yaml-cronjob-${cronJob.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${cronJob.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="cronjob"
                    namespace={cronJob.metadata.namespace}
                    resourceName={cronJob.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleRunNow = async (cronJob) => {
        try {
            const name = cronJob.metadata.name;
            const namespace = cronJob.metadata.namespace;

            Logger.info("Run now requested for CronJob", { namespace, name });
            Logger.info("About to call TriggerCronJob function");

            await TriggerCronJob(namespace, name);

            Logger.info("TriggerCronJob returned successfully", { namespace, name });
            triggerRefresh();
        } catch (err) {
            Logger.error("Failed to trigger CronJob", { error: err, message: err?.message, stack: err?.stack });
            alert(`Failed to trigger cronjob: ${err.message || err}`);
        }
    };

    const handleSuspend = async (cronJob) => {
        try {
            const isSuspended = cronJob.spec?.suspend || false;
            const action = isSuspended ? "Resume" : "Suspend";
            const name = cronJob.metadata.name;
            const namespace = cronJob.metadata.namespace;
            const newSuspendValue = !isSuspended;

            Logger.info(`${action} requested for CronJob`, { namespace, name, currentSuspend: isSuspended, newSuspend: newSuspendValue });
            Logger.info(`About to call SuspendCronJob function`);

            const result = await SuspendCronJob(namespace, name, newSuspendValue);

            Logger.info(`SuspendCronJob returned`, { result });
            Logger.info(`CronJob ${action.toLowerCase()}d successfully`, { namespace, name });
            triggerRefresh();
        } catch (err) {
            const action = cronJob.spec?.suspend ? "resume" : "suspend";
            Logger.error(`Failed to ${action} CronJob`, { error: err, message: err?.message, stack: err?.stack });
            alert(`Failed to ${action} cronjob: ${err.message || err}`);
        }
    };

    const handleDelete = async (cronJob) => {
        const name = cronJob.metadata.name;
        const namespace = cronJob.metadata.namespace;

        if (!window.confirm(`Are you sure you want to delete cronjob "${name}"? This will also delete all associated jobs.`)) {
            return;
        }

        try {
            Logger.info("Delete CronJob requested", { namespace, name });
            Logger.info("About to call DeleteCronJob function");

            await DeleteCronJob(namespace, name);

            Logger.info("DeleteCronJob returned successfully", { namespace, name });
            triggerRefresh();
        } catch (err) {
            Logger.error("Failed to delete CronJob", { error: err, message: err?.message, stack: err?.stack });
            alert(`Failed to delete cronjob: ${err.message || err}`);
        }
    };

    return {
        handleViewLogs,
        handleEditYaml,
        handleRunNow,
        handleSuspend,
        handleDelete
    };
};
