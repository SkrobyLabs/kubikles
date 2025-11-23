import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteConfigMap, LogDebug } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';

export const useConfigMapActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (configMap) => {
        const tabId = `yaml-configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `YAML: ${configMap.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={configMap.metadata.namespace}
                    podName={configMap.metadata.name}
                    isConfigMap={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = async (configMap) => {
        if (!confirm(`Are you sure you want to delete configmap ${configMap.metadata.name}?`)) return;

        const msg = `Deleting configmap: ${configMap.metadata.name}`;
        console.log(msg);
        try {
            await LogDebug(msg);
            await DeleteConfigMap(configMap.metadata.namespace, configMap.metadata.name);
            console.log("Delete triggered successfully");
        } catch (err) {
            console.error("Failed to delete configmap", err);
            alert(`Failed to delete configmap: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
