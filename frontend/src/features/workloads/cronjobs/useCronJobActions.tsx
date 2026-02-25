import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteCronJob, TriggerCronJob, SuspendCronJob, ListJobs, ListPods } from 'wailsjs/go/main/App';
import { useK8s } from '~/context';
import CronJobDetails from '~/components/shared/CronJobDetails';
import { DeferredLogViewer, ResolvedLogViewerProps } from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sCronJob } from '~/types/k8s';

interface CronJobActionsReturn extends BaseResourceActionsReturn<K8sCronJob> {
    handleViewLogs: (cronJob: K8sCronJob) => void;
    handleRunNow: (cronJob: K8sCronJob) => Promise<void>;
    handleSuspend: (cronJob: K8sCronJob) => Promise<void>;
    handleDelete: (cronJob: K8sCronJob) => void;
}

export const useCronJobActions = (): any => {
    const { triggerRefresh } = useK8s();
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        openTab,
        openModal,
        closeModal,

        addNotification,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'cronjob',
        resourceLabel: 'CronJob',
        DetailsComponent: CronJobDetails,
        detailsPropName: 'cronJob',
    });

    const handleViewLogs = (cronJob: K8sCronJob): void => {
        Logger.info("View logs for CronJob", { namespace: cronJob.metadata.namespace, name: cronJob.metadata.name }, 'k8s');
        const namespace = cronJob.metadata.namespace;

        openTab({
            id: `logs-cronjob-${cronJob.metadata.name}`,
            title: `Logs: ${cronJob.metadata.name}`,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const allJobs = await ListJobs('', namespace);
                        const cronJobJobs = allJobs.filter((job: any) => {
                            const ownerRefs = job.metadata?.ownerReferences || [];
                            return ownerRefs.some((ref: any) =>
                                ref.kind === 'CronJob' && ref.name === cronJob.metadata.name
                            );
                        });

                        if (cronJobJobs.length === 0) return null;

                        cronJobJobs.sort((a: any, b: any) =>
                            new Date(b.metadata.creationTimestamp).getTime() - new Date(a.metadata.creationTimestamp).getTime()
                        );
                        const mostRecentJob = cronJobJobs[0];

                        const allPods = await ListPods('', namespace);
                        const jobPods = allPods.filter((pod: any) =>
                            pod.metadata?.labels?.['job-name'] === mostRecentJob.metadata.name
                        );

                        if (jobPods.length === 0) return null;

                        const pod = jobPods[0];
                        const containers = [
                            ...(pod.spec?.initContainers || []).map((c: any) => c.name),
                            ...(pod.spec?.containers || []).map((c: any) => c.name)
                        ];

                        const podContainerMap: Record<string, string[]> = {};
                        for (const p of jobPods) {
                            podContainerMap[p.metadata.name] = [
                                ...(p.spec?.initContainers || []).map((c: any) => c.name),
                                ...(p.spec?.containers || []).map((c: any) => c.name)
                            ];
                        }

                        return {
                            namespace,
                            pod: pod.metadata.name,
                            containers,
                            siblingPods: jobPods.map((p: any) => p.metadata.name),
                            podContainerMap,
                            ownerName: cronJob.metadata.name,
                        };
                    }}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'CronJob', name: cronJob.metadata.name, namespace },
        });
    };

    const handleRunNow = async (cronJob: K8sCronJob): Promise<void> => {
        try {
            const name = cronJob.metadata.name;
            const namespace = cronJob.metadata.namespace;
            Logger.info("Run now requested for CronJob", { namespace, name }, 'k8s');
            await TriggerCronJob(namespace, name);
            Logger.info("TriggerCronJob returned successfully", { namespace, name }, 'k8s');
            triggerRefresh();
        } catch (err: any) {
            Logger.error("Failed to trigger CronJob", err, 'k8s');
            addNotification({ type: 'error', title: 'Failed to trigger cronjob', message: String(err.message || err) });
        }
    };

    const handleSuspend = async (cronJob: K8sCronJob): Promise<void> => {
        try {
            const isSuspended = cronJob.spec?.suspend || false;
            const action = isSuspended ? "Resume" : "Suspend";
            const name = cronJob.metadata.name;
            const namespace = cronJob.metadata.namespace;

            Logger.info(`${action} requested for CronJob`, { namespace, name }, 'k8s');
            await SuspendCronJob(namespace, name, !isSuspended);
            Logger.info(`CronJob ${action.toLowerCase()}d successfully`, { namespace, name }, 'k8s');
            triggerRefresh();
        } catch (err: any) {
            const action = cronJob.spec?.suspend ? "resume" : "suspend";
            Logger.error(`Failed to ${action} CronJob`, err, 'k8s');
            addNotification({ type: 'error', title: `Failed to ${action} cronjob`, message: String(err.message || err) });
        }
    };

    const handleDelete = (cronJob: K8sCronJob): void => {
        const name = cronJob.metadata.name;
        const namespace = cronJob.metadata.namespace;
        Logger.info("Delete CronJob requested", { namespace, name }, 'k8s');

        openModal({
            title: `Delete CronJob ${name}?`,
            content: `Are you sure you want to delete cronjob "${name}"? This will also delete all associated jobs.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteCronJob(namespace, name);
                    Logger.info("CronJob deleted successfully", { namespace, name }, 'k8s');
                    closeModal();
                    triggerRefresh();
                } catch (err: any) {
                    Logger.error("Failed to delete CronJob", err, 'k8s');
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
