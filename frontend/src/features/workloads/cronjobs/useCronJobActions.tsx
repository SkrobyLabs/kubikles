import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteCronJob, TriggerCronJob, SuspendCronJob, ListJobs, ListPods } from 'wailsjs/go/main/App';
import { useK8s } from '~/context';
import CronJobDetails from '~/components/shared/CronJobDetails';
import LogViewer from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sCronJob } from '~/types/k8s';

interface CronJobActionsReturn extends BaseResourceActionsReturn<K8sCronJob> {
    handleViewLogs: (cronJob: K8sCronJob) => Promise<void>;
    handleRunNow: (cronJob: K8sCronJob) => Promise<void>;
    handleSuspend: (cronJob: K8sCronJob) => Promise<void>;
    handleDelete: (cronJob: K8sCronJob) => void;
}

export const useCronJobActions = (): CronJobActionsReturn => {
    const { triggerRefresh } = useK8s();
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        openTab,
        openModal,
        closeModal,

        addNotification,
    } = useBaseResourceActions({
        resourceType: 'cronjob',
        resourceLabel: 'CronJob',
        DetailsComponent: CronJobDetails,
        detailsPropName: 'cronJob',
    });

    const handleViewLogs = async (cronJob: K8sCronJob): Promise<void> => {
        Logger.info("View logs for CronJob", { namespace: cronJob.metadata.namespace, name: cronJob.metadata.name });
        const namespace = cronJob.metadata.namespace;

        try {
            const allJobs = await ListJobs('', namespace);
            const cronJobJobs = allJobs.filter(job => {
                const ownerRefs = job.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'CronJob' && ref.name === cronJob.metadata.name
                );
            });

            if (cronJobJobs.length === 0) {
                addNotification({ type: 'warning', title: 'No jobs found', message: `No jobs found for cronjob "${cronJob.metadata.name}". The cronjob may not have run yet.` });
                return;
            }

            cronJobJobs.sort((a, b) =>
                new Date(b.metadata.creationTimestamp) - new Date(a.metadata.creationTimestamp)
            );
            const mostRecentJob = cronJobJobs[0];

            const allPods = await ListPods('', namespace);
            const jobPods = allPods.filter(pod =>
                pod.metadata?.labels?.['job-name'] === mostRecentJob.metadata.name
            );

            if (jobPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for job "${mostRecentJob.metadata.name}".` });
                return;
            }

            const pod = jobPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            const podContainerMap = {};
            for (const p of jobPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            openTab({
                id: `logs-cronjob-${cronJob.metadata.name}`,
                title: `Logs: ${cronJob.metadata.name}`,
                keepAlive: true,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={jobPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={cronJob.metadata.name}
                        tabContext={currentContext}
                    />
                ),
                resourceMeta: { kind: 'CronJob', name: cronJob.metadata.name, namespace },
            });
        } catch (err) {
            Logger.error("Failed to get logs for CronJob", err);
            addNotification({ type: 'error', title: 'Failed to get logs for cronjob', message: String(err.message || err) });
        }
    };

    const handleRunNow = async (cronJob: K8sCronJob): Promise<void> => {
        try {
            const name = cronJob.metadata.name;
            const namespace = cronJob.metadata.namespace;
            Logger.info("Run now requested for CronJob", { namespace, name });
            await TriggerCronJob(namespace, name);
            Logger.info("TriggerCronJob returned successfully", { namespace, name });
            triggerRefresh();
        } catch (err) {
            Logger.error("Failed to trigger CronJob", err);
            addNotification({ type: 'error', title: 'Failed to trigger cronjob', message: String(err.message || err) });
        }
    };

    const handleSuspend = async (cronJob: K8sCronJob): Promise<void> => {
        try {
            const isSuspended = cronJob.spec?.suspend || false;
            const action = isSuspended ? "Resume" : "Suspend";
            const name = cronJob.metadata.name;
            const namespace = cronJob.metadata.namespace;

            Logger.info(`${action} requested for CronJob`, { namespace, name });
            await SuspendCronJob(namespace, name, !isSuspended);
            Logger.info(`CronJob ${action.toLowerCase()}d successfully`, { namespace, name });
            triggerRefresh();
        } catch (err) {
            const action = cronJob.spec?.suspend ? "resume" : "suspend";
            Logger.error(`Failed to ${action} CronJob`, err);
            addNotification({ type: 'error', title: `Failed to ${action} cronjob`, message: String(err.message || err) });
        }
    };

    const handleDelete = (cronJob: K8sCronJob): void => {
        const name = cronJob.metadata.name;
        const namespace = cronJob.metadata.namespace;
        Logger.info("Delete CronJob requested", { namespace, name });

        openModal({
            title: `Delete CronJob ${name}?`,
            content: `Are you sure you want to delete cronjob "${name}"? This will also delete all associated jobs.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteCronJob(namespace, name);
                    Logger.info("CronJob deleted successfully", { namespace, name });
                    closeModal();
                    triggerRefresh();
                } catch (err) {
                    Logger.error("Failed to delete CronJob", err);
                    addNotification({ type: 'error', title: 'Failed to delete cronjob', message: String(err.message || err) });
                }
            }
        });
    };

    return {
        handleShowDetails,
        handleViewLogs,
        handleEditYaml,
        handleShowDependencies,
        handleRunNow,
        handleSuspend,
        handleDelete
    };
};
