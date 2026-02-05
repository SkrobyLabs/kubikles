import React from 'react';
import { useUI } from '../../../context';
import { useK8s } from '../../../context';
import { useNotification } from '../../../context';
import { DeleteRoleBinding } from '../../../../wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '../../../components/lazy';
import Logger from '../../../utils/Logger';

export const useRoleBindingActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

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
            ),
            resourceMeta: { kind: 'RoleBinding', name: roleBinding.metadata.name, namespace: roleBinding.metadata.namespace },
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
                    await DeleteRoleBinding(namespace, name);
                    Logger.info("RoleBinding deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete RoleBinding", err);
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
