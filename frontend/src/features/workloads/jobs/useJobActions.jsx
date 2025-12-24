import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteJob, ListPods } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import LogViewer from '../../../components/shared/log-viewer';
import Logger from '../../../utils/Logger';

export const useJobActions = (onRefresh) => {
    const { openTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (job) => {
        const namespace = job.metadata.namespace;
        Logger.info("Opening YAML editor for Job", { namespace, name: job.metadata.name });
        openTab({
            id: `job-yaml-${job.metadata.namespace}-${job.metadata.name}`,
            title: `Edit: ${job.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="job"
                    namespace={namespace}
                    resourceName={job.metadata.name}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (job) => {
        Logger.info("Opening dependency graph", { namespace: job.metadata.namespace, job: job.metadata.name });
        openTab({
            id: `deps-job-${job.metadata.uid}`,
            title: `Deps: ${job.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="job"
                    namespace={job.metadata.namespace}
                    resourceName={job.metadata.name}
                />
            )
        });
    };

    const handleDelete = async (job) => {
        const namespace = job.metadata.namespace;
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
        const namespace = job.metadata.namespace;
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
                        tabContext={currentContext}
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
        handleShowDependencies,
        handleDelete,
        handleViewLogs
    };
};
