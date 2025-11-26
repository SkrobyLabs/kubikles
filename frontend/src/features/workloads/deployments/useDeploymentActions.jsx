import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteDeployment, RestartDeployment, ListPods } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import LogViewer from '../../../components/shared/LogViewer';
import Logger from '../../../utils/Logger';

export const useDeploymentActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext, currentNamespace } = useK8s();

    const handleEditYaml = (deployment) => {
        Logger.info("Opening YAML editor", { namespace: deployment.metadata.namespace, deployment: deployment.metadata.name });
        const tabId = `yaml-deploy-${deployment.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${deployment.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="deployment"
                    namespace={deployment.metadata.namespace}
                    resourceName={deployment.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (deployment) => {
        Logger.info("Opening dependency graph", { namespace: deployment.metadata.namespace, deployment: deployment.metadata.name });
        const tabId = `deps-deploy-${deployment.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${deployment.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="deployment"
                    namespace={deployment.metadata.namespace}
                    resourceName={deployment.metadata.name}
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
        Logger.info("Delete Deployment requested", { namespace: deployment.metadata.namespace, name: deployment.metadata.name });
        const name = deployment.metadata.name;
        const namespace = deployment.metadata.namespace;

        openModal({
            title: `Delete Deployment ${name}?`,
            content: `Are you sure you want to delete deployment "${name}"? This will also delete all associated pods.`,
            onConfirm: async () => {
                try {
                    Logger.info("Deleting Deployment", { namespace, name });
                    await DeleteDeployment(currentContext, namespace, name);
                    Logger.info("Deployment deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete Deployment", err);
                    alert(`Failed to delete deployment: ${err.message || err}`);
                }
            }
        });
    };

    const handleViewLogs = async (deployment) => {
        Logger.info("View logs for Deployment", { namespace: deployment.metadata.namespace, name: deployment.metadata.name });
        const namespace = deployment.metadata.namespace;

        try {
            const allPods = await ListPods(namespace);

            // Filter pods that belong to this deployment
            const deploymentPods = allPods.filter(pod => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'ReplicaSet' &&
                    ref.name?.startsWith(deployment.metadata.name + '-')
                );
            });

            if (deploymentPods.length === 0) {
                Logger.info("No pods found for Deployment", { namespace, name: deployment.metadata.name });
                alert(`No pods found for deployment "${deployment.metadata.name}".`);
                return;
            }

            const pod = deploymentPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            // Build container map for all pods
            const podContainerMap = {};
            for (const p of deploymentPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            Logger.info("Opening logs for Deployment pod", {
                namespace,
                deployment: deployment.metadata.name,
                pod: pod.metadata.name,
                totalPods: deploymentPods.length
            });

            const tabId = `logs-deploy-${deployment.metadata.name}`;
            openTab({
                id: tabId,
                title: `Logs: ${deployment.metadata.name}`,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={deploymentPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={deployment.metadata.name}
                        tabContext={currentContext}
                    />
                )
            });
        } catch (err) {
            Logger.error("Failed to get pods for Deployment", err);
            alert(`Failed to get pods for deployment: ${err.message || err}`);
        }
    };

    return {
        handleEditYaml,
        handleShowDependencies,
        handleRestart,
        handleDelete,
        handleViewLogs
    };
};
