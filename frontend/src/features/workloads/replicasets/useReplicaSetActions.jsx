import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteReplicaSet, ListPods } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import ReplicaSetDetails from '../../../components/shared/ReplicaSetDetails';
import LogViewer from '../../../components/shared/log-viewer';
import Logger from '../../../utils/Logger';

export const useReplicaSetActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleShowDetails = (replicaSet) => {
        Logger.info("Opening ReplicaSet details", { namespace: replicaSet.metadata.namespace, name: replicaSet.metadata.name });
        const tabId = `details-replicaset-${replicaSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${replicaSet.metadata.name}`,
            content: (
                <ReplicaSetDetails
                    replicaSet={replicaSet}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleEditYaml = (replicaSet) => {
        Logger.info("Opening YAML editor for ReplicaSet", { namespace: replicaSet.metadata.namespace, name: replicaSet.metadata.name });
        const tabId = `edit-rs-${replicaSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${replicaSet.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="replicaset"
                    namespace={replicaSet.metadata.namespace}
                    resourceName={replicaSet.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (replicaSet) => {
        Logger.info("Opening dependency graph", { namespace: replicaSet.metadata.namespace, replicaSet: replicaSet.metadata.name });
        const tabId = `deps-replicaset-${replicaSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${replicaSet.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="replicaset"
                    namespace={replicaSet.metadata.namespace}
                    resourceName={replicaSet.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = async (replicaSet) => {
        Logger.info("Delete ReplicaSet requested", { namespace: replicaSet.metadata.namespace, name: replicaSet.metadata.name });
        const name = replicaSet.metadata.name;
        const namespace = replicaSet.metadata.namespace;

        openModal({
            title: `Delete ReplicaSet ${name}?`,
            content: `Are you sure you want to delete replicaset "${name}"? This will also delete all associated pods.`,
            onConfirm: async () => {
                try {
                    Logger.info("Deleting ReplicaSet", { namespace, name });
                    await DeleteReplicaSet(currentContext, namespace, name);
                    Logger.info("ReplicaSet deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete ReplicaSet", err);
                    alert(`Failed to delete replicaset: ${err.message || err}`);
                }
            }
        });
    };

    const handleViewLogs = async (replicaSet) => {
        Logger.info("View logs for ReplicaSet", { namespace: replicaSet.metadata.namespace, name: replicaSet.metadata.name });
        const namespace = replicaSet.metadata.namespace;

        try {
            const allPods = await ListPods(namespace);

            // Filter pods that belong to this replicaset
            const replicaSetPods = allPods.filter(pod => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'ReplicaSet' &&
                    ref.name === replicaSet.metadata.name
                );
            });

            if (replicaSetPods.length === 0) {
                Logger.info("No pods found for ReplicaSet", { namespace, name: replicaSet.metadata.name });
                alert(`No pods found for replicaset "${replicaSet.metadata.name}".`);
                return;
            }

            const pod = replicaSetPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            // Build container map for all pods
            const podContainerMap = {};
            for (const p of replicaSetPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            Logger.info("Opening logs for ReplicaSet pod", {
                namespace,
                replicaSet: replicaSet.metadata.name,
                pod: pod.metadata.name,
                totalPods: replicaSetPods.length
            });

            const tabId = `logs-replicaset-${replicaSet.metadata.name}`;
            openTab({
                id: tabId,
                title: `Logs: ${replicaSet.metadata.name}`,
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
            alert(`Failed to get pods for replicaset: ${err.message || err}`);
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
