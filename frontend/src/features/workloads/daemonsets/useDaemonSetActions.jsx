import React from 'react';
import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteDaemonSet, RestartDaemonSet, ListPods } from '../../../../wailsjs/go/main/App';
import DaemonSetDetails from '../../../components/shared/DaemonSetDetails';
import LogViewer from '../../../components/shared/log-viewer';
import Logger from '../../../utils/Logger';

export const useDaemonSetActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,
        currentContext,
        addNotification,
    } = useBaseResourceActions({
        resourceType: 'daemonset',
        resourceLabel: 'DaemonSet',
        DetailsComponent: DaemonSetDetails,
        detailsPropName: 'daemonSet',
    });

    const handleRestart = async (daemonSet) => {
        Logger.info("Restart DaemonSet requested", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name });
        try {
            await RestartDaemonSet(currentContext, daemonSet.metadata.namespace, daemonSet.metadata.name);
            Logger.info("DaemonSet restarted successfully", { name: daemonSet.metadata.name });
        } catch (err) {
            Logger.error("Failed to restart DaemonSet", err);
            addNotification({ type: 'error', title: 'Failed to restart daemonset', message: String(err.message || err) });
        }
    };

    const handleDelete = createDeleteHandler(
        async (daemonSet) => {
            await DeleteDaemonSet(currentContext, daemonSet.metadata.namespace, daemonSet.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this daemonset? This will also delete all associated pods.' }
    );

    const handleViewLogs = async (daemonSet) => {
        Logger.info("View logs for DaemonSet", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name });
        const namespace = daemonSet.metadata.namespace;

        try {
            const allPods = await ListPods(namespace);
            const daemonSetPods = allPods.filter(pod => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'DaemonSet' && ref.name === daemonSet.metadata.name
                );
            });

            if (daemonSetPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for daemonset "${daemonSet.metadata.name}".` });
                return;
            }

            const pod = daemonSetPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            const podContainerMap = {};
            for (const p of daemonSetPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            openTab({
                id: `logs-daemonset-${daemonSet.metadata.name}`,
                title: `Logs: ${daemonSet.metadata.name}`,
                keepAlive: true,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={daemonSetPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={daemonSet.metadata.name}
                        tabContext={currentContext}
                    />
                )
            });
        } catch (err) {
            Logger.error("Failed to get pods for DaemonSet", err);
            addNotification({ type: 'error', title: 'Failed to get pods for daemonset', message: String(err.message || err) });
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
