import { useUI } from '../../../context/UIContext';
import { DeleteJob, ListPods } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import LogViewer from '../../../components/shared/LogViewer';
import Logger from '../../../utils/Logger';

export const useJobActions = (namespace, onRefresh) => {
    const { openTab, openModal, closeModal } = useUI();

    const handleEditYaml = (job) => {
        Logger.info("Opening YAML editor for Job", { namespace, name: job.metadata.name });
        openTab({
            id: `job-yaml-${job.metadata.name}`,
            title: `Edit: ${job.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={namespace}
                    resourceName={job.metadata.name}
                    isJob={true}
                />
            )
        });
    };

    const handleDelete = async (job) => {
        Logger.info("Delete Job requested", { namespace, name: job.metadata.name });
        openModal({
            title: 'Confirm Delete',
            content: `Are you sure you want to delete job "${job.metadata.name}"?`,
            onConfirm: async () => {
                try {
                    Logger.info("Deleting Job", { namespace, name: job.metadata.name });
                    await DeleteJob(namespace, job.metadata.name);
                    Logger.info("Job deleted successfully", { namespace, name: job.metadata.name });
                    closeModal();
                    if (onRefresh) onRefresh();
                } catch (err) {
                    Logger.error("Failed to delete Job", err);
                    alert(`Failed to delete job: ${err.message || err}`);
                }
            }
        });
    };

    const handleViewLogs = async (job) => {
        Logger.info("View logs for Job", { namespace, name: job.metadata.name });

        try {
            // Query for pods created by this job
            const allPods = await ListPods(namespace);

            // Filter pods that belong to this job
            // Jobs create pods with labels: job-name=<job-name>
            const jobPods = allPods.filter(pod =>
                pod.metadata?.labels?.['job-name'] === job.metadata.name
            );

            if (jobPods.length === 0) {
                Logger.info("No pods found for Job", { namespace, name: job.metadata.name });
                alert(`No pods found for job "${job.metadata.name}". The job may not have created any pods yet.`);
                return;
            }

            // Get the first pod (or most recent)
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

            Logger.info("Opening logs for Job pod", {
                namespace,
                job: job.metadata.name,
                pod: pod.metadata.name,
                totalPods: jobPods.length
            });

            const tabId = `logs-job-${job.metadata.name}`;
            openTab({
                id: tabId,
                title: `Logs: ${job.metadata.name}`,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={jobPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={job.metadata.name}
                    />
                )
            });
        } catch (err) {
            Logger.error("Failed to get pods for Job", err);
            alert(`Failed to get pods for job: ${err.message || err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete,
        handleViewLogs
    };
};
