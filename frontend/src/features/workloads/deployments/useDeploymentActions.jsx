import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteDeployment, RestartDeployment, LogDebug } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';

export const useDeploymentActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (deployment) => {
        const tabId = `yaml-deploy-${deployment.metadata.uid}`;
        openTab({
            id: tabId,
            title: `YAML: ${deployment.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={deployment.metadata.namespace}
                    podName={deployment.metadata.name}
                    isDeployment={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleRestart = async (deployment) => {
        const msg = `Restarting deployment: ${deployment.metadata.name}`;
        console.log(msg);
        try {
            await LogDebug(msg);
            await RestartDeployment(currentContext, deployment.metadata.namespace, deployment.metadata.name);
            console.log("Restart triggered successfully");
        } catch (err) {
            console.error("Failed to restart deployment", err);
            alert(`Failed to restart deployment: ${err}`);
        }
    };

    const handleDelete = async (deployment) => {
        if (!confirm(`Are you sure you want to delete deployment ${deployment.metadata.name}?`)) return;

        const msg = `Deleting deployment: ${deployment.metadata.name}`;
        console.log(msg);
        try {
            await LogDebug(msg);
            await DeleteDeployment(currentContext, deployment.metadata.namespace, deployment.metadata.name);
            console.log("Delete triggered successfully");
            // We rely on the watcher or re-fetch in the component
        } catch (err) {
            console.error("Failed to delete deployment", err);
            alert(`Failed to delete deployment: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleRestart,
        handleDelete
    };
};
