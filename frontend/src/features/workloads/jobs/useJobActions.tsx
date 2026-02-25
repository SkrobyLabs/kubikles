import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteJob, ListPods } from 'wailsjs/go/main/App';
import JobDetails from '~/components/shared/JobDetails';
import { DeferredLogViewer, ResolvedLogViewerProps } from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sJob } from '~/types/k8s';

interface JobActionsReturn extends BaseResourceActionsReturn<K8sJob> {
    handleDelete: (job: K8sJob) => void;
    handleViewLogs: (job: K8sJob) => void;
}

export const useJobActions = (onRefresh?: () => void): any => {
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
        resourceType: 'job',
        resourceLabel: 'Job',
        DetailsComponent: JobDetails,
        detailsPropName: 'job',
    });

    const handleDelete = (job: K8sJob): void => {
        const namespace = job.metadata.namespace;
        Logger.info("Delete Job requested", { namespace, name: job.metadata.name }, 'k8s');
        openModal({
            title: 'Confirm Delete',
            content: `Are you sure you want to delete job "${job.metadata.name}"?`,
            onConfirm: async () => {
                try {
                    await DeleteJob(namespace, job.metadata.name);
                    Logger.info("Job deleted successfully", { namespace, name: job.metadata.name }, 'k8s');
                    closeModal();
                    if (onRefresh) onRefresh();
                } catch (err: any) {
                    Logger.error("Failed to delete Job", err, 'k8s');
                    addNotification({ type: 'error', title: 'Failed to delete job', message: String(err.message || err) });
                }
            }
        });
    };

    const handleViewLogs = (job: K8sJob): void => {
        const namespace = job.metadata.namespace;
        Logger.info("View logs for Job", { namespace, name: job.metadata.name }, 'k8s');

        openTab({
            id: `logs-job-${job.metadata.name}`,
            title: `Logs: ${job.metadata.name}`,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const allPods = await ListPods('', namespace);
                        const jobPods = allPods.filter((pod: any) =>
                            pod.metadata?.labels?.['job-name'] === job.metadata.name
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
                            ownerName: job.metadata.name,
                        };
                    }}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'Job', name: job.metadata.name, namespace },
        });
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete,
        handleViewLogs
    };
};
