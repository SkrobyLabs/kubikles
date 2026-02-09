import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteStatefulSet, RestartStatefulSet, ListPods } from 'wailsjs/go/main/App';
import StatefulSetDetails from '~/components/shared/StatefulSetDetails';
import LogViewer from '~/components/shared/log-viewer';
import Logger from '~/utils/Logger';
import { K8sStatefulSet, K8sPod } from '~/types/k8s';

export interface StatefulSetActionsReturn extends BaseResourceActionsReturn<K8sStatefulSet> {
    handleRestart: (statefulSet: K8sStatefulSet) => Promise<void>;
    handleDelete: (statefulSet: K8sStatefulSet) => void;
    handleViewLogs: (statefulSet: K8sStatefulSet) => Promise<void>;
}

export const useStatefulSetActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        openTab,

        addNotification,
        currentContext,
    } = useBaseResourceActions<K8sStatefulSet>({
        resourceType: 'statefulset',
        resourceLabel: 'StatefulSet',
        DetailsComponent: StatefulSetDetails,
        detailsPropName: 'statefulSet',
    });

    const handleRestart = async (statefulSet: K8sStatefulSet): Promise<void> => {
        Logger.info("Restarting statefulset", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name }, 'k8s');
        try {
            await RestartStatefulSet(statefulSet.metadata.namespace!, statefulSet.metadata.name);
            Logger.info("Restart triggered successfully", { name: statefulSet.metadata.name }, 'k8s');
        } catch (err: any) {
            Logger.error("Failed to restart statefulset", err, 'k8s');
            addNotification({ type: 'error', title: 'Failed to restart statefulset', message: String(err) });
        }
    };

    const handleDelete = createDeleteHandler(
        async (statefulSet: any): Promise<void> => {
            await DeleteStatefulSet(statefulSet.metadata.namespace!, statefulSet.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this statefulset? This will also delete all associated pods.' }
    );

    const handleViewLogs = async (statefulSet: K8sStatefulSet): Promise<void> => {
        Logger.info("View logs for StatefulSet", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name }, 'k8s');
        const namespace = statefulSet.metadata.namespace!;

        try {
            const allPods: K8sPod[] = await ListPods('', namespace);
            const statefulSetPods = allPods.filter((pod: K8sPod) => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some((ref: any) =>
                    ref.kind === 'StatefulSet' && ref.name === statefulSet.metadata.name
                );
            });

            if (statefulSetPods.length === 0) {
                addNotification({ type: 'warning', title: 'No pods found', message: `No pods found for statefulset "${statefulSet.metadata.name}".` });
                return;
            }

            const pod = statefulSetPods[0];
            const containers: string[] = [
                ...(pod.spec?.initContainers || []).map((c: any) => c.name),
                ...(pod.spec?.containers || []).map((c: any) => c.name)
            ];

            const podContainerMap: Record<string, string[]> = {};
            for (const p of statefulSetPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map((c: any) => c.name),
                    ...(p.spec?.containers || []).map((c: any) => c.name)
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
                        siblingPods={statefulSetPods.map((p: any) => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={statefulSet.metadata.name}
                        tabContext={currentContext}
                    />
                ),
                resourceMeta: { kind: 'StatefulSet', name: statefulSet.metadata.name, namespace },
            });
        } catch (err: any) {
            Logger.error("Failed to get pods for StatefulSet", err, 'k8s');
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
