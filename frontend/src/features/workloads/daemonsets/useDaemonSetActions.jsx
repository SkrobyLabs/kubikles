import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { GetDaemonSetYaml, UpdateDaemonSetYaml, RestartDaemonSet, DeleteDaemonSet } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';

export function useDaemonSetActions() {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { refresh } = useK8s();

    const handleEditYaml = (daemonSet) => {
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
        try {
            await RestartDaemonSet(namespace, name);
            refresh();
        } catch (err) {
            console.error(`Failed to restart daemonset ${name}:`, err);
        }
    };

    const handleDelete = (namespace, name) => {
        openModal({
            id: `delete-ds-${name}`,
            type: 'confirmation',
            title: `Delete DaemonSet ${name}?`,
            message: `Are you sure you want to delete DaemonSet "${name}"? This action cannot be undone.`,
            confirmLabel: 'Delete',
            cancelLabel: 'Cancel',
            onConfirm: async () => {
                try {
                    await DeleteDaemonSet(namespace, name);
                    refresh();
                    closeModal();
                } catch (err) {
                    console.error(`Failed to delete daemonset ${name}:`, err);
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
