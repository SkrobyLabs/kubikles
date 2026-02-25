import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteReplicaSet, ListPods } from 'wailsjs/go/main/App';
import ReplicaSetDetails from '~/components/shared/ReplicaSetDetails';
import { DeferredLogViewer, ResolvedLogViewerProps } from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sReplicaSet, K8sPod } from '~/types/k8s';

export interface ReplicaSetActionsReturn extends BaseResourceActionsReturn<K8sReplicaSet> {
    handleDelete: (replicaSet: K8sReplicaSet) => void;
    handleViewLogs: (replicaSet: K8sReplicaSet) => void;
}

export const useReplicaSetActions = (): any => {
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
        async (replicaSet: any): Promise<void> => {
            await DeleteReplicaSet(replicaSet.metadata.namespace!, replicaSet.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this replicaset? This will also delete all associated pods.' }
    );

    const handleViewLogs = (replicaSet: K8sReplicaSet): void => {
        Logger.info("View logs for ReplicaSet", { namespace: replicaSet.metadata.namespace, name: replicaSet.metadata.name }, 'k8s');
        const namespace = replicaSet.metadata.namespace!;

        openTab({
            id: `logs-replicaset-${replicaSet.metadata.name}`,
            title: `Logs: ${replicaSet.metadata.name}`,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const allPods: K8sPod[] = await ListPods('', namespace);
                        const replicaSetPods: K8sPod[] = allPods.filter((pod: K8sPod) => {
                            const ownerRefs = pod.metadata?.ownerReferences || [];
                            return ownerRefs.some((ref: any) =>
                                ref.kind === 'ReplicaSet' && ref.name === replicaSet.metadata.name
                            );
                        });

                        if (replicaSetPods.length === 0) return null;

                        const pod: K8sPod = replicaSetPods[0];
                        const containers: string[] = [
                            ...(pod.spec?.initContainers || []).map((c: any) => c.name),
                            ...(pod.spec?.containers || []).map((c: any) => c.name)
                        ];

                        const podContainerMap: Record<string, string[]> = {};
                        for (const p of replicaSetPods) {
                            podContainerMap[p.metadata.name] = [
                                ...(p.spec?.initContainers || []).map((c: any) => c.name),
                                ...(p.spec?.containers || []).map((c: any) => c.name)
                            ];
                        }

                        return {
                            namespace,
                            pod: pod.metadata.name,
                            containers,
                            siblingPods: replicaSetPods.map((p: any) => p.metadata.name),
                            podContainerMap,
                            ownerName: replicaSet.metadata.name,
                        };
                    }}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'ReplicaSet', name: replicaSet.metadata.name, namespace },
        });
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

        addNotification,
        createDeleteHandler,
    };
};
