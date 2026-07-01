import React from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteStatefulSet, RestartStatefulSet, ListPods } from 'wailsjs/go/main/App';
import StatefulSetDetails from '~/components/shared/StatefulSetDetails';
import { DeferredLogViewer, ResolvedLogViewerProps } from '~/components/shared/log-viewer';
import { resolveLogTargetFromPods } from '~/components/shared/log-viewer/logTarget';
import Logger from '~/utils/Logger';
import { K8sStatefulSet, K8sPod } from '~/types/k8s';

export interface StatefulSetActionsReturn extends BaseResourceActionsReturn<K8sStatefulSet> {
    handleRestart: (statefulSet: K8sStatefulSet) => Promise<void>;
    handleDelete: (statefulSet: K8sStatefulSet) => void;
    handleViewLogs: (statefulSet: K8sStatefulSet) => void;
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

    const handleViewLogs = (statefulSet: K8sStatefulSet): void => {
        Logger.info("View logs for StatefulSet", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name }, 'k8s');
        const namespace = statefulSet.metadata.namespace!;
        const refreshToken = Date.now();

        openTab({
            id: `logs-statefulset-${currentContext}-${namespace}/${statefulSet.metadata.name}`,
            title: `Logs: ${statefulSet.metadata.name}`,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    refreshToken={refreshToken}
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const allPods: K8sPod[] = await ListPods('', namespace);
                        const statefulSetPods = allPods.filter((pod: K8sPod) => {
                            const ownerRefs = pod.metadata?.ownerReferences || [];
                            return ownerRefs.some((ref: any) =>
                                ref.kind === 'StatefulSet' && ref.name === statefulSet.metadata.name
                            );
                        });

                        if (statefulSetPods.length === 0) return null;

                        return resolveLogTargetFromPods(namespace, statefulSetPods, statefulSet.metadata.name);
                    }}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'StatefulSet', name: statefulSet.metadata.name, namespace },
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
