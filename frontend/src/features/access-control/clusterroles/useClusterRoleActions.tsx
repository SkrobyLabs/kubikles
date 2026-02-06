import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { DeleteClusterRole } from 'wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '~/components/lazy';
import Logger from '~/utils/Logger';
import { K8sClusterRole } from '~/types/k8s';

interface ClusterRoleActionsReturn {
    handleEditYaml: (clusterRole: K8sClusterRole) => void;
    handleDelete: (clusterRole: K8sClusterRole) => void;
}

export const useClusterRoleActions = (): any => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (clusterRole: K8sClusterRole): void => {
        Logger.info("Opening ClusterRole editor", { name: clusterRole.metadata.name });
        const tabId = `clusterrole-${clusterRole.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${clusterRole.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="clusterrole"
                    resourceName={clusterRole.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'ClusterRole', name: clusterRole.metadata.name },
        });
    };

    const handleDelete = (clusterRole: K8sClusterRole): void => {
        const name = clusterRole.metadata.name;
        Logger.info("Delete ClusterRole requested", { name });

        openModal({
            title: `Delete ClusterRole ${name}?`,
            content: `Are you sure you want to delete cluster role "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await DeleteClusterRole(name);
                    Logger.info("ClusterRole deleted successfully", { name });
                    closeModal();
                } catch (err: any) {
                    Logger.error("Failed to delete ClusterRole", err);
                    addNotification({ type: 'error', title: 'Failed to delete cluster role', message: String(err) });
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
