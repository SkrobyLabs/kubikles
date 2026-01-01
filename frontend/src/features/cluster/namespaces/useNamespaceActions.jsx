import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteNamespace } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import NamespaceDetails from '../../../components/shared/NamespaceDetails';
import Logger from '../../../utils/Logger';

export const useNamespaceActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleShowDetails = (namespace) => {
        Logger.info("Opening namespace details", { namespace: namespace.metadata.name });
        const tabId = `details-namespace-${namespace.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${namespace.metadata.name}`,
            content: (
                <NamespaceDetails
                    namespace={namespace}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleEditYaml = (namespace) => {
        Logger.info("Opening namespace YAML editor", { namespace: namespace.metadata.name });
        const tabId = `yaml-namespace-${namespace.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${namespace.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="namespace"
                    resourceName={namespace.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleDelete = (namespace) => {
        const name = namespace.metadata.name;
        Logger.info("Delete Namespace requested", { name });

        // Warn about system namespaces
        const systemNamespaces = ['default', 'kube-system', 'kube-public', 'kube-node-lease'];
        const isSystemNamespace = systemNamespaces.includes(name);

        openModal({
            title: `Delete Namespace ${name}?`,
            content: isSystemNamespace
                ? `WARNING: "${name}" is a system namespace. Deleting it may cause cluster issues!\n\nThis will delete ALL resources within this namespace!`
                : `Are you sure you want to delete namespace "${name}"?\n\nThis will delete ALL resources within this namespace!`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteNamespace(name);
                    Logger.info("Namespace deleted successfully", { name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete namespace", err);
                    alert(`Failed to delete namespace: ${err}`);
                }
            }
        });
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
