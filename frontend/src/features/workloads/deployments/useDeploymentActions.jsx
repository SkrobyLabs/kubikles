import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteDeployment, RestartDeployment } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useDeploymentActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (deployment) => {
        Logger.info("Opening YAML editor", { namespace: deployment.metadata.namespace, deployment: deployment.metadata.name });
        const tabId = `yaml-deploy-${deployment.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${deployment.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={deployment.metadata.namespace}
                    resourceName={deployment.metadata.name}
                    isDeployment={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleRestart = async (deployment) => {
        Logger.info("Restarting deployment", { namespace: deployment.metadata.namespace, name: deployment.metadata.name });
        try {
            await RestartDeployment(currentContext, deployment.metadata.namespace, deployment.metadata.name);
            Logger.info("Restart triggered successfully", { name: deployment.metadata.name });
        } catch (err) {
            Logger.error("Failed to restart deployment", err);
            alert(`Failed to restart deployment: ${err}`);
        }
    };

    const handleDelete = async (deployment) => {
        if (!confirm(`Are you sure you want to delete deployment ${deployment.metadata.name}?`)) return;

        Logger.info("Deleting deployment", { namespace: deployment.metadata.namespace, name: deployment.metadata.name });
        try {
            await DeleteDeployment(currentContext, deployment.metadata.namespace, deployment.metadata.name);
            Logger.info("Delete triggered successfully", { name: deployment.metadata.name });
        } catch (err) {
            Logger.error("Failed to delete deployment", err);
            alert(`Failed to delete deployment: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleRestart,
        handleDelete
    };
};
