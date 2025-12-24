import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteClusterRole } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useClusterRoleActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (clusterRole) => {
        Logger.info("Opening ClusterRole editor", { name: clusterRole.metadata.name });
        const tabId = `clusterrole-${clusterRole.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${clusterRole.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="clusterrole"
                    resourceName={clusterRole.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleDelete = (clusterRole) => {
        const name = clusterRole.metadata.name;
        Logger.info("Delete ClusterRole requested", { name });

        openModal({
            title: `Delete ClusterRole ${name}?`,
            content: `Are you sure you want to delete cluster role "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteClusterRole(currentContext, name);
                    Logger.info("ClusterRole deleted successfully", { name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete ClusterRole", err);
                    alert(`Failed to delete cluster role: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
