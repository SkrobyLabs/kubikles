import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteRoleBinding } from '../../../../wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '../../../components/lazy';
import Logger from '../../../utils/Logger';

export const useRoleBindingActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (roleBinding) => {
        Logger.info("Opening RoleBinding editor", { namespace: roleBinding.metadata.namespace, name: roleBinding.metadata.name });
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
            )
        });
    };

    const handleDelete = (roleBinding) => {
        const name = roleBinding.metadata.name;
        const namespace = roleBinding.metadata.namespace;
        Logger.info("Delete RoleBinding requested", { namespace, name });

        openModal({
            title: `Delete RoleBinding ${name}?`,
            content: `Are you sure you want to delete role binding "${name}" in namespace "${namespace}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteRoleBinding(currentContext, namespace, name);
                    Logger.info("RoleBinding deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete RoleBinding", err);
                    alert(`Failed to delete role binding: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
