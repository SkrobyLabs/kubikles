import React from 'react';
import { useUI } from '../../../context';
import { useK8s } from '../../../context';
import { useNotification } from '../../../context';
import { DeleteClusterRoleBinding } from '../../../../wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '../../../components/lazy';
import Logger from '../../../utils/Logger';
import { K8sClusterRoleBinding } from '../../../types/k8s';
import { BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';

/**
 * Return type for useClusterRoleBindingActions
 */
export interface ClusterRoleBindingActionsReturn extends Pick<BaseResourceActionsReturn<K8sClusterRoleBinding>, 'handleEditYaml'> {
    handleDelete: (clusterRoleBinding: K8sClusterRoleBinding) => void;
}

export const useClusterRoleBindingActions = (): ClusterRoleBindingActionsReturn => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (clusterRoleBinding: K8sClusterRoleBinding): void => {
        Logger.info("Opening ClusterRoleBinding editor", { name: clusterRoleBinding.metadata.name });
        const tabId = `clusterrolebinding-${clusterRoleBinding.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${clusterRoleBinding.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="clusterrolebinding"
                    resourceName={clusterRoleBinding.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'ClusterRoleBinding', name: clusterRoleBinding.metadata.name },
        });
    };

    const handleDelete = (clusterRoleBinding: K8sClusterRoleBinding): void => {
        const name = clusterRoleBinding.metadata.name;
        Logger.info("Delete ClusterRoleBinding requested", { name });

        openModal({
            title: `Delete ClusterRoleBinding ${name}?`,
            content: `Are you sure you want to delete cluster role binding "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await DeleteClusterRoleBinding(name);
                    Logger.info("ClusterRoleBinding deleted successfully", { name });
                    closeModal();
                } catch (err: any) {
                    Logger.error("Failed to delete ClusterRoleBinding", err);
                    addNotification({ type: 'error', title: 'Failed to delete cluster role binding', message: String(err) });
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
