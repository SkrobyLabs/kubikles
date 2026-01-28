import React from 'react';
import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteStatefulSet, RestartStatefulSet, ListPods } from '../../../../wailsjs/go/main/App';
import StatefulSetDetails from '../../../components/shared/StatefulSetDetails';
import LogViewer from '../../../components/shared/log-viewer';
import Logger from '../../../utils/Logger';

export const useStatefulSetActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,
        currentContext,
        addNotification,
    } = useBaseResourceActions({
        resourceType: 'statefulset',
        resourceLabel: 'StatefulSet',
        DetailsComponent: StatefulSetDetails,
        detailsPropName: 'statefulSet',
    });

    const handleRestart = async (statefulSet) => {
        Logger.info("Restarting statefulset", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name });
        try {
            await RestartStatefulSet(statefulSet.metadata.namespace, statefulSet.metadata.name);
            Logger.info("Restart triggered successfully", { name: statefulSet.metadata.name });
        } catch (err) {
            Logger.error("Failed to restart statefulset", err);
            addNotification({ type: 'error', title: 'Failed to restart statefulset', message: String(err) });
        }
    };

    const handleDelete = createDeleteHandler(
        async (statefulSet) => {
            await DeleteStatefulSet(statefulSet.metadata.namespace, statefulSet.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this statefulset? This will also delete all associated pods.' }
    );

    const handleViewLogs = async (statefulSet) => {
        Logger.info("View logs for StatefulSet", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name });
        const namespace = statefulSet.metadata.namespace;

        try {
            const allPods = await ListPods(namespace);
            const statefulSetPods = allPods.filter(pod => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'StatefulSet' && ref.name === statefulSet.metadata.name
                );
            });

            if (statefulSetPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for statefulset "${statefulSet.metadata.name}".` });
                return;
            }

            const pod = statefulSetPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            const podContainerMap = {};
            for (const p of statefulSetPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            openTab({
                id: `logs-statefulset-${statefulSet.metadata.name}`,
                title: `Logs: ${statefulSet.metadata.name}`,
                keepAlive: true,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={statefulSetPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={statefulSet.metadata.name}
                        tabContext={currentContext}
                    />
                )
            });
        } catch (err) {
            Logger.error("Failed to get pods for StatefulSet", err);
            addNotification({ type: 'error', title: 'Failed to get pods for statefulset', message: String(err.message || err) });
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
