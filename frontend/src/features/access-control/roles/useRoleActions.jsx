import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteRole } from '../../../../wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '../../../components/lazy';
import Logger from '../../../utils/Logger';

export const useRoleActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (role) => {
        Logger.info("Opening Role editor", { namespace: role.metadata.namespace, name: role.metadata.name });
        const tabId = `role-${role.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${role.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="role"
                    namespace={role.metadata.namespace}
                    resourceName={role.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleDelete = (role) => {
        const name = role.metadata.name;
        const namespace = role.metadata.namespace;
        Logger.info("Delete Role requested", { namespace, name });

        openModal({
            title: `Delete Role ${name}?`,
            content: `Are you sure you want to delete role "${name}" in namespace "${namespace}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteRole(currentContext, namespace, name);
                    Logger.info("Role deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete Role", err);
                    alert(`Failed to delete role: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
