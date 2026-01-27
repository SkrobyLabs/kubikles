import React from 'react';
import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteReplicaSet, ListPods } from '../../../../wailsjs/go/main/App';
import ReplicaSetDetails from '../../../components/shared/ReplicaSetDetails';
import LogViewer from '../../../components/shared/log-viewer';
import Logger from '../../../utils/Logger';

export const useReplicaSetActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,
        currentContext,
        addNotification,
    } = useBaseResourceActions({
        resourceType: 'replicaset',
        resourceLabel: 'ReplicaSet',
        DetailsComponent: ReplicaSetDetails,
        detailsPropName: 'replicaSet',
    });

    const handleDelete = createDeleteHandler(
        async (replicaSet) => {
            await DeleteReplicaSet(currentContext, replicaSet.metadata.namespace, replicaSet.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this replicaset? This will also delete all associated pods.' }
    );

    const handleViewLogs = async (replicaSet) => {
        Logger.info("View logs for ReplicaSet", { namespace: replicaSet.metadata.namespace, name: replicaSet.metadata.name });
        const namespace = replicaSet.metadata.namespace;

        try {
            const allPods = await ListPods(namespace);
            const replicaSetPods = allPods.filter(pod => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'ReplicaSet' && ref.name === replicaSet.metadata.name
                );
            });

            if (replicaSetPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for replicaset "${replicaSet.metadata.name}".` });
                return;
            }

            const pod = replicaSetPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            const podContainerMap = {};
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
                )
            });
        } catch (err) {
            Logger.error("Failed to get pods for ReplicaSet", err);
            addNotification({ type: 'error', title: 'Failed to get pods for replicaset', message: String(err.message || err) });
        }
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete,
        handleViewLogs
    };
};
