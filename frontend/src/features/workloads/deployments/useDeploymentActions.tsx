import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteDeployment, RestartDeployment, ListPods } from 'wailsjs/go/main/App';
import DeploymentDetails from '~/components/shared/DeploymentDetails';
import LogViewer from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sDeployment, K8sPod } from '~/types/k8s';

export interface DeploymentActionsReturn extends BaseResourceActionsReturn<K8sDeployment> {
    handleRestart: (deployment: K8sDeployment) => Promise<void>;
    handleDelete: (deployment: K8sDeployment) => void;
    handleViewLogs: (deployment: K8sDeployment) => Promise<void>;
}

export const useDeploymentActions = (): DeploymentActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,

        addNotification,
    } = useBaseResourceActions({
        resourceType: 'deployment',
        resourceLabel: 'Deployment',
        DetailsComponent: DeploymentDetails,
        detailsPropName: 'deployment',
    });

    const handleRestart = async (deployment: K8sDeployment): Promise<void> => {
        Logger.info("Restarting deployment", { namespace: deployment.metadata.namespace, name: deployment.metadata.name });
        try {
            await RestartDeployment(deployment.metadata.namespace, deployment.metadata.name);
            Logger.info("Restart triggered successfully", { name: deployment.metadata.name });
        } catch (err) {
            Logger.error("Failed to restart deployment", err);
            addNotification({ type: 'error', title: 'Failed to restart deployment', message: String(err) });
        }
    };

    const handleDelete = createDeleteHandler(
        async (deployment: K8sDeployment): Promise<void> => {
            await DeleteDeployment(deployment.metadata.namespace, deployment.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this deployment? This will also delete all associated pods.' }
    );

    const handleViewLogs = async (deployment: K8sDeployment): Promise<void> => {
        Logger.info("View logs for Deployment", { namespace: deployment.metadata.namespace, name: deployment.metadata.name });
        const namespace = deployment.metadata.namespace;

        try {
            const allPods: K8sPod[] = await ListPods('', namespace);
            const deploymentPods: K8sPod[] = allPods.filter((pod: K8sPod) => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'ReplicaSet' &&
                    ref.name?.startsWith(deployment.metadata.name + '-')
                );
            });

            if (deploymentPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for deployment "${deployment.metadata.name}".` });
                return;
            }

            const pod: K8sPod = deploymentPods[0];
            const containers: string[] = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            const podContainerMap: Record<string, string[]> = {};
            for (const p of deploymentPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            openTab({
                id: `logs-deploy-${deployment.metadata.name}`,
                title: `Logs: ${deployment.metadata.name}`,
                keepAlive: true,
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
                ),
                resourceMeta: { kind: 'Deployment', name: deployment.metadata.name, namespace },
            });
        } catch (err: any) {
            Logger.error("Failed to get pods for Deployment", err);
            addNotification({ type: 'error', title: 'Failed to get pods for deployment', message: String(err.message || err) });
        }
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleRestart,
        handleDelete,
        handleViewLogs
    };
};
