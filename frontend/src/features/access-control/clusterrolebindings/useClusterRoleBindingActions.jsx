import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteClusterRoleBinding } from '../../../../wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '../../../components/lazy';
import Logger from '../../../utils/Logger';

export const useClusterRoleBindingActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (clusterRoleBinding) => {
        Logger.info("Opening ClusterRoleBinding editor", { name: clusterRoleBinding.metadata.name });
        const tabId = `clusterrolebinding-${clusterRoleBinding.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${clusterRoleBinding.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="clusterrolebinding"
                    resourceName={clusterRoleBinding.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleDelete = (clusterRoleBinding) => {
        const name = clusterRoleBinding.metadata.name;
        Logger.info("Delete ClusterRoleBinding requested", { name });

        openModal({
            title: `Delete ClusterRoleBinding ${name}?`,
            content: `Are you sure you want to delete cluster role binding "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteClusterRoleBinding(currentContext, name);
                    Logger.info("ClusterRoleBinding deleted successfully", { name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete ClusterRoleBinding", err);
                    alert(`Failed to delete cluster role binding: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
