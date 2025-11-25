import React from 'react';
import { useUI } from '../../../context/UIContext';
import { DeleteNamespace } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useNamespaceActions = () => {
    const { openTab, closeTab } = useUI();

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
                />
            )
        });
    };

    const handleDelete = async (namespace) => {
        const name = namespace.metadata.name;

        // Warn about system namespaces
        const systemNamespaces = ['default', 'kube-system', 'kube-public', 'kube-node-lease'];
        if (systemNamespaces.includes(name)) {
            alert(`Warning: "${name}" is a system namespace. Deleting it may cause cluster issues.`);
        }

        if (!confirm(`Are you sure you want to delete namespace "${name}"?\n\nThis will delete ALL resources within this namespace!`)) return;

        Logger.info("Deleting namespace", { name });
        try {
            await DeleteNamespace(name);
            Logger.info("Delete triggered successfully", { name });
        } catch (err) {
            Logger.error("Failed to delete namespace", err);
            alert(`Failed to delete namespace: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
