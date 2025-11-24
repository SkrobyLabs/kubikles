import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { GetReplicaSetYaml, UpdateReplicaSetYaml, DeleteReplicaSet } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export function useReplicaSetActions() {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { refresh } = useK8s();

    const handleEditYaml = (replicaSet) => {
        Logger.info("Opening YAML editor", { namespace: replicaSet.metadata.namespace, replicaSet: replicaSet.metadata.name });
        const tabId = `edit-rs-${replicaSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit ${replicaSet.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={replicaSet.metadata.namespace}
                    podName={replicaSet.metadata.name}
                    isReplicaSet={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = (namespace, name) => {
        Logger.info("Requesting delete replicaset", { namespace, name });
        openModal({
            id: `delete-rs-${name}`,
            type: 'confirmation',
            title: `Delete ReplicaSet ${name}?`,
            message: `Are you sure you want to delete ReplicaSet "${name}"? This action cannot be undone.`,
            confirmLabel: 'Delete',
            cancelLabel: 'Cancel',
            onConfirm: async () => {
                Logger.info("Confirming delete replicaset", { namespace, name });
                try {
                    await DeleteReplicaSet(namespace, name);
                    Logger.info("ReplicaSet deleted successfully", { name });
                    refresh();
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete replicaset", err);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
}
