import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import { DeleteJob, ListPods } from '../../../../wailsjs/go/main/App';
import JobDetails from '../../../components/shared/JobDetails';
import LogViewer from '../../../components/shared/log-viewer';
import Logger from '../../../utils/Logger';
import { K8sJob } from '../../../types/k8s';

interface JobActionsReturn extends BaseResourceActionsReturn<K8sJob> {
    handleDelete: (job: K8sJob) => void;
    handleViewLogs: (job: K8sJob) => Promise<void>;
}

export const useJobActions = (onRefresh?: () => void): JobActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        openTab,
        openModal,
        closeModal,
        currentContext,
        addNotification,
    } = useBaseResourceActions({
        resourceType: 'job',
        resourceLabel: 'Job',
        DetailsComponent: JobDetails,
        detailsPropName: 'job',
    });

    const handleDelete = (job: K8sJob): void => {
        const namespace = job.metadata.namespace;
        Logger.info("Delete Job requested", { namespace, name: job.metadata.name });
        openModal({
            title: 'Confirm Delete',
            content: `Are you sure you want to delete job "${job.metadata.name}"?`,
            onConfirm: async () => {
                try {
                    await DeleteJob(namespace, job.metadata.name);
                    Logger.info("Job deleted successfully", { namespace, name: job.metadata.name });
                    closeModal();
                    if (onRefresh) onRefresh();
                } catch (err) {
                    Logger.error("Failed to delete Job", err);
                    addNotification({ type: 'error', title: 'Failed to delete job', message: String(err.message || err) });
                }
            }
        });
    };

    const handleViewLogs = async (job: K8sJob): Promise<void> => {
        const namespace = job.metadata.namespace;
        Logger.info("View logs for Job", { namespace, name: job.metadata.name });

        try {
            const allPods = await ListPods('', namespace);
            const jobPods = allPods.filter(pod =>
                pod.metadata?.labels?.['job-name'] === job.metadata.name
            );

            if (jobPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for job "${job.metadata.name}". The job may not have created any pods yet.` });
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
                id: `logs-job-${job.metadata.name}`,
                title: `Logs: ${job.metadata.name}`,
                keepAlive: true,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={jobPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={job.metadata.name}
                        tabContext={currentContext}
                    />
                ),
                resourceMeta: { kind: 'Job', name: job.metadata.name, namespace },
            });
        } catch (err) {
            Logger.error("Failed to get pods for Job", err);
            addNotification({ type: 'error', title: 'Failed to get pods for job', message: String(err.message || err) });
        }
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete,
        handleViewLogs
    };
};
