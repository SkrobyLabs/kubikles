import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { GetDaemonSetYaml, UpdateDaemonSetYaml, RestartDaemonSet, DeleteDaemonSet } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export function useDaemonSetActions() {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { refresh } = useK8s();

    const handleEditYaml = (daemonSet) => {
        Logger.info("Opening YAML editor", { namespace: daemonSet.metadata.namespace, daemonSet: daemonSet.metadata.name });
        const tabId = `edit-ds-${daemonSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit ${daemonSet.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={daemonSet.metadata.namespace}
                    podName={daemonSet.metadata.name}
                    isDaemonSet={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleRestart = async (namespace, name) => {
        Logger.info("Restarting daemonset", { namespace, name });
        try {
            await RestartDaemonSet(namespace, name);
            Logger.info("Restart triggered successfully", { name });
            refresh();
        } catch (err) {
            Logger.error("Failed to restart daemonset", err);
        }
    };

    const handleDelete = (namespace, name) => {
        Logger.info("Requesting delete daemonset", { namespace, name });
        openModal({
            id: `delete-ds-${name}`,
            type: 'confirmation',
            title: `Delete DaemonSet ${name}?`,
            message: `Are you sure you want to delete DaemonSet "${name}"? This action cannot be undone.`,
            confirmLabel: 'Delete',
            cancelLabel: 'Cancel',
            onConfirm: async () => {
                Logger.info("Confirming delete daemonset", { namespace, name });
                try {
                    await DeleteDaemonSet(namespace, name);
                    Logger.info("DaemonSet deleted successfully", { name });
                    refresh();
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete daemonset", err);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleRestart,
        handleDelete
    };
}
