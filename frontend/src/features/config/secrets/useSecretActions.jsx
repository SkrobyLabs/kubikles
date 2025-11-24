import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteSecret } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useSecretActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (secret) => {
        Logger.info("Opening YAML editor", { namespace: secret.metadata.namespace, secret: secret.metadata.name });
        const tabId = `yaml-secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `YAML: ${secret.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={secret.metadata.namespace}
                    podName={secret.metadata.name}
                    isSecret={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = async (secret) => {
        if (!confirm(`Are you sure you want to delete secret ${secret.metadata.name}?`)) return;

        Logger.info("Deleting secret", { namespace: secret.metadata.namespace, name: secret.metadata.name });
        try {
            await DeleteSecret(secret.metadata.namespace, secret.metadata.name);
            Logger.info("Delete triggered successfully", { name: secret.metadata.name });
        } catch (err) {
            Logger.error("Failed to delete secret", err);
            alert(`Failed to delete secret: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
