import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteReplicaSet, ListPods } from 'wailsjs/go/main/App';
import ReplicaSetDetails from '~/components/shared/ReplicaSetDetails';
import { DeferredLogViewer, ResolvedLogViewerProps } from '~/components/shared/log-viewer';
import { resolveLogTargetFromPods } from '~/components/shared/log-viewer/logTarget';
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
        const refreshToken = Date.now();

        openTab({
            id: `logs-replicaset-${replicaSet.metadata.name}`,
            title: `Logs: ${replicaSet.metadata.name}`,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    refreshToken={refreshToken}
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const allPods: K8sPod[] = await ListPods('', namespace);
                        const replicaSetPods: K8sPod[] = allPods.filter((pod: K8sPod) => {
                            const ownerRefs = pod.metadata?.ownerReferences || [];
                            return ownerRefs.some((ref: any) =>
                                ref.kind === 'ReplicaSet' && ref.name === replicaSet.metadata.name
                            );
                        });

                        if (replicaSetPods.length === 0) return null;

                        return resolveLogTargetFromPods(namespace, replicaSetPods, replicaSet.metadata.name);
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
