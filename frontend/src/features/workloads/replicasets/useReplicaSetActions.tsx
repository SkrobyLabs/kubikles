import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import { DeleteReplicaSet, ListPods } from '../../../../wailsjs/go/main/App';
import ReplicaSetDetails from '../../../components/shared/ReplicaSetDetails';
import LogViewer from '../../../components/shared/log-viewer';
import Logger from '../../../utils/Logger';
import { K8sReplicaSet, K8sPod } from '../../../types/k8s';

export interface ReplicaSetActionsReturn extends BaseResourceActionsReturn<K8sReplicaSet> {
    handleDelete: (replicaSet: K8sReplicaSet) => void;
    handleViewLogs: (replicaSet: K8sReplicaSet) => Promise<void>;
}

export const useReplicaSetActions = (): ReplicaSetActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,
        closeTab,
        openModal,
        closeModal,
        currentContext,
        addNotification,
    } = useBaseResourceActions<K8sReplicaSet>({
        resourceType: 'replicaset',
        resourceLabel: 'ReplicaSet',
        DetailsComponent: ReplicaSetDetails,
        detailsPropName: 'replicaSet',
    });

    const handleDelete = createDeleteHandler(
        async (replicaSet: K8sReplicaSet): Promise<void> => {
            await DeleteReplicaSet(replicaSet.metadata.namespace!, replicaSet.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this replicaset? This will also delete all associated pods.' }
    );

    const handleViewLogs = async (replicaSet: K8sReplicaSet): Promise<void> => {
        Logger.info("View logs for ReplicaSet", { namespace: replicaSet.metadata.namespace, name: replicaSet.metadata.name });
        const namespace = replicaSet.metadata.namespace!;

        try {
            const allPods: K8sPod[] = await ListPods('', namespace);
            const replicaSetPods: K8sPod[] = allPods.filter((pod: K8sPod) => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'ReplicaSet' && ref.name === replicaSet.metadata.name
                );
            });

            if (replicaSetPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for replicaset "${replicaSet.metadata.name}".` });
                return;
            }

            const pod: K8sPod = replicaSetPods[0];
            const containers: string[] = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            const podContainerMap: Record<string, string[]> = {};
            for (const p of replicaSetPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            openTab({
                id: `logs-replicaset-${replicaSet.metadata.name}`,
                title: `Logs: ${replicaSet.metadata.name}`,
                keepAlive: true,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={replicaSetPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={replicaSet.metadata.name}
                        tabContext={currentContext}
                    />
                ),
                resourceMeta: { kind: 'ReplicaSet', name: replicaSet.metadata.name, namespace },
            });
        } catch (err: any) {
            Logger.error("Failed to get pods for ReplicaSet", err);
            addNotification({ type: 'error', title: 'Failed to get pods for replicaset', message: String(err.message || err) });
        }
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete,
        handleViewLogs,
        openTab,
        closeTab,
        openModal,
        closeModal,
        currentContext,
        addNotification,
        createDeleteHandler,
    };
};
