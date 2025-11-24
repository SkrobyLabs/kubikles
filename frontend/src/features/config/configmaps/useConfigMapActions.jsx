import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteConfigMap } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useConfigMapActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (configMap) => {
        Logger.info("Opening YAML editor", { namespace: configMap.metadata.namespace, configMap: configMap.metadata.name });
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

        Logger.info("Deleting configmap", { namespace: configMap.metadata.namespace, name: configMap.metadata.name });
        try {
            await DeleteConfigMap(configMap.metadata.namespace, configMap.metadata.name);
            Logger.info("Delete triggered successfully", { name: configMap.metadata.name });
        } catch (err) {
            Logger.error("Failed to delete configmap", err);
            alert(`Failed to delete configmap: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
