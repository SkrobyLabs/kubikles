import { useUI } from '../../../context/UIContext';
import { DeleteJob } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
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

    const handleViewLogs = (job) => {
        // Jobs create pods, so we'll open logs for the first pod if available
        // This is a simplified approach - in reality, you might want to list pods by job selector
        Logger.info("View logs for Job (opening first pod logs)", { namespace, name: job.metadata.name });

        // For now, we'll just log that this action was triggered
        // In a full implementation, you'd query for pods with the job's selector
        // and open logs for the first pod
        alert("Job logs: This would open logs for the job's pods. Implementation pending.");
    };

    return {
        handleEditYaml,
        handleDelete,
        handleViewLogs
    };
};
