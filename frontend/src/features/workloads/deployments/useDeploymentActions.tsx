import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteDeployment, RestartDeployment, ListPods } from 'wailsjs/go/main/App';
import DeploymentDetails from '~/components/shared/DeploymentDetails';
import { DeferredLogViewer, ResolvedLogViewerProps } from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sDeployment, K8sPod } from '~/types/k8s';

export interface DeploymentActionsReturn extends BaseResourceActionsReturn<K8sDeployment> {
    handleRestart: (deployment: K8sDeployment) => Promise<void>;
    handleDelete: (deployment: K8sDeployment) => void;
    handleViewLogs: (deployment: K8sDeployment) => void;
}

export const useDeploymentActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,

        addNotification,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'deployment',
        resourceLabel: 'Deployment',
        DetailsComponent: DeploymentDetails,
        detailsPropName: 'deployment',
    });

    const handleRestart = async (deployment: K8sDeployment): Promise<void> => {
        Logger.info("Restarting deployment", { namespace: deployment.metadata.namespace, name: deployment.metadata.name }, 'k8s');
        try {
            await RestartDeployment(deployment.metadata.namespace, deployment.metadata.name);
            Logger.info("Restart triggered successfully", { name: deployment.metadata.name }, 'k8s');
        } catch (err: any) {
            Logger.error("Failed to restart deployment", err, 'k8s');
            addNotification({ type: 'error', title: 'Failed to restart deployment', message: String(err) });
        }
    };

    const handleDelete = createDeleteHandler(
        async (deployment: any): Promise<void> => {
            await DeleteDeployment(deployment.metadata.namespace, deployment.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this deployment? This will also delete all associated pods.' }
    );

    const handleViewLogs = (deployment: K8sDeployment): void => {
        Logger.info("View logs for Deployment", { namespace: deployment.metadata.namespace, name: deployment.metadata.name }, 'k8s');
        const namespace = deployment.metadata.namespace;

        openTab({
            id: `logs-deploy-${deployment.metadata.name}`,
            title: `Logs: ${deployment.metadata.name}`,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const allPods: K8sPod[] = await ListPods('', namespace);
                        const deploymentPods: K8sPod[] = allPods.filter((pod: K8sPod) => {
                            const ownerRefs = pod.metadata?.ownerReferences || [];
                            return ownerRefs.some((ref: any) =>
                                ref.kind === 'ReplicaSet' &&
                                ref.name?.startsWith(deployment.metadata.name + '-')
                            );
                        });

                        if (deploymentPods.length === 0) return null;

                        const pod: K8sPod = deploymentPods[0];
                        const containers: string[] = [
                            ...(pod.spec?.initContainers || []).map((c: any) => c.name),
                            ...(pod.spec?.containers || []).map((c: any) => c.name)
                        ];

                        const podContainerMap: Record<string, string[]> = {};
                        for (const p of deploymentPods) {
                            podContainerMap[p.metadata.name] = [
                                ...(p.spec?.initContainers || []).map((c: any) => c.name),
                                ...(p.spec?.containers || []).map((c: any) => c.name)
                            ];
                        }

                        return {
                            namespace,
                            pod: pod.metadata.name,
                            containers,
                            siblingPods: deploymentPods.map((p: any) => p.metadata.name),
                            podContainerMap,
                            ownerName: deployment.metadata.name,
                        };
                    }}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'Deployment', name: deployment.metadata.name, namespace },
        });
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
