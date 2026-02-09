import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { DeleteRoleBinding } from 'wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '~/components/lazy';
import Logger from '~/utils/Logger';
import { K8sRoleBinding } from '~/types/k8s';

interface RoleBindingActionsReturn {
    handleEditYaml: (roleBinding: K8sRoleBinding) => void;
    handleDelete: (roleBinding: K8sRoleBinding) => void;
}

export const useRoleBindingActions = (): any => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (roleBinding: K8sRoleBinding): void => {
        Logger.info("Opening RoleBinding editor", { namespace: roleBinding.metadata.namespace, name: roleBinding.metadata.name }, 'k8s');
        const tabId = `rolebinding-${roleBinding.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${roleBinding.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="rolebinding"
                    namespace={roleBinding.metadata.namespace}
                    resourceName={roleBinding.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'RoleBinding', name: roleBinding.metadata.name, namespace: roleBinding.metadata.namespace },
        });
    };

    const handleDelete = (roleBinding: K8sRoleBinding): void => {
        const name = roleBinding.metadata.name;
        const namespace = roleBinding.metadata.namespace;
        Logger.info("Delete RoleBinding requested", { namespace, name }, 'k8s');

        openModal({
            title: `Delete RoleBinding ${name}?`,
            content: `Are you sure you want to delete role binding "${name}" in namespace "${namespace}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await DeleteRoleBinding(namespace, name);
                    Logger.info("RoleBinding deleted successfully", { namespace, name }, 'k8s');
                    closeModal();
                } catch (err: any) {
                    Logger.error("Failed to delete RoleBinding", err, 'k8s');
                    addNotification({ type: 'error', title: 'Failed to delete role binding', message: String(err) });
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
