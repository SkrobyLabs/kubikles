import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteDaemonSet, RestartDaemonSet, ListPods } from 'wailsjs/go/main/App';
import DaemonSetDetails from '~/components/shared/DaemonSetDetails';
import { DeferredLogViewer, ResolvedLogViewerProps } from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sDaemonSet } from '~/types/k8s';

interface DaemonSetActionsReturn extends BaseResourceActionsReturn<K8sDaemonSet> {
    handleRestart: (daemonSet: K8sDaemonSet) => Promise<void>;
    handleDelete: (daemonSet: K8sDaemonSet) => void;
    handleViewLogs: (daemonSet: K8sDaemonSet) => void;
}

export const useDaemonSetActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,

        addNotification,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'daemonset',
        resourceLabel: 'DaemonSet',
        DetailsComponent: DaemonSetDetails,
        detailsPropName: 'daemonSet',
    });

    const handleRestart = async (daemonSet: K8sDaemonSet): Promise<void> => {
        Logger.info("Restart DaemonSet requested", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name }, 'k8s');
        try {
            await RestartDaemonSet(daemonSet.metadata.namespace, daemonSet.metadata.name);
            Logger.info("DaemonSet restarted successfully", { name: daemonSet.metadata.name }, 'k8s');
        } catch (err: any) {
            Logger.error("Failed to restart DaemonSet", err, 'k8s');
            addNotification({ type: 'error', title: 'Failed to restart daemonset', message: String(err.message || err) });
        }
    };

    const handleDelete = createDeleteHandler(
        async (daemonSet: any): Promise<void> => {
            await DeleteDaemonSet(daemonSet.metadata.namespace, daemonSet.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this daemonset? This will also delete all associated pods.' }
    );

    const handleViewLogs = (daemonSet: K8sDaemonSet): void => {
        Logger.info("View logs for DaemonSet", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name }, 'k8s');
        const namespace = daemonSet.metadata.namespace;

        openTab({
            id: `logs-daemonset-${daemonSet.metadata.name}`,
            title: `Logs: ${daemonSet.metadata.name}`,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const allPods = await ListPods('', namespace);
                        const daemonSetPods = allPods.filter((pod: any) => {
                            const ownerRefs = pod.metadata?.ownerReferences || [];
                            return ownerRefs.some((ref: any) =>
                                ref.kind === 'DaemonSet' && ref.name === daemonSet.metadata.name
                            );
                        });

                        if (daemonSetPods.length === 0) return null;

                        const pod = daemonSetPods[0];
                        const containers = [
                            ...(pod.spec?.initContainers || []).map((c: any) => c.name),
                            ...(pod.spec?.containers || []).map((c: any) => c.name)
                        ];

                        const podContainerMap: Record<string, string[]> = {};
                        for (const p of daemonSetPods) {
                            podContainerMap[p.metadata.name] = [
                                ...(p.spec?.initContainers || []).map((c: any) => c.name),
                                ...(p.spec?.containers || []).map((c: any) => c.name)
                            ];
                        }

                        return {
                            namespace,
                            pod: pod.metadata.name,
                            containers,
                            siblingPods: daemonSetPods.map((p: any) => p.metadata.name),
                            podContainerMap,
                            ownerName: daemonSet.metadata.name,
                        };
                    }}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'DaemonSet', name: daemonSet.metadata.name, namespace },
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
