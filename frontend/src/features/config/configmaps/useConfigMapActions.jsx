import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteConfigMap } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import Logger from '../../../utils/Logger';

export const useConfigMapActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (configMap) => {
        Logger.info("Opening YAML editor", { namespace: configMap.metadata.namespace, configMap: configMap.metadata.name });
        const tabId = `yaml-configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${configMap.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="configmap"
                    namespace={configMap.metadata.namespace}
                    resourceName={configMap.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (configMap) => {
        Logger.info("Opening dependency graph", { namespace: configMap.metadata.namespace, configMap: configMap.metadata.name });
        const tabId = `deps-configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${configMap.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="configmap"
                    namespace={configMap.metadata.namespace}
                    resourceName={configMap.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = (configMap) => {
        const name = configMap.metadata.name;
        const namespace = configMap.metadata.namespace;
        Logger.info("Delete ConfigMap requested", { namespace, name });

        openModal({
            title: `Delete ConfigMap ${name}?`,
            content: `Are you sure you want to delete configmap "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteConfigMap(namespace, name);
                    Logger.info("ConfigMap deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete configmap", err);
                    alert(`Failed to delete configmap: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
