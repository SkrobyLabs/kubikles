import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteStatefulSet, RestartStatefulSet, ListPods } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import LogViewer from '../../../components/shared/LogViewer';
import Logger from '../../../utils/Logger';

export const useStatefulSetActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (statefulSet) => {
        Logger.info("Opening YAML editor", { namespace: statefulSet.metadata.namespace, statefulSet: statefulSet.metadata.name });
        const tabId = `yaml-statefulset-${statefulSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${statefulSet.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="statefulset"
                    namespace={statefulSet.metadata.namespace}
                    resourceName={statefulSet.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleRestart = async (statefulSet) => {
        Logger.info("Restarting statefulset", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name });
        try {
            await RestartStatefulSet(currentContext, statefulSet.metadata.namespace, statefulSet.metadata.name);
            Logger.info("Restart triggered successfully", { name: statefulSet.metadata.name });
        } catch (err) {
            Logger.error("Failed to restart statefulset", err);
            alert(`Failed to restart statefulset: ${err}`);
        }
    };

    const handleDelete = async (statefulSet) => {
        Logger.info("Delete StatefulSet requested", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name });
        const name = statefulSet.metadata.name;
        const namespace = statefulSet.metadata.namespace;

        openModal({
            title: `Delete StatefulSet ${name}?`,
            content: `Are you sure you want to delete statefulset "${name}"? This will also delete all associated pods.`,
            onConfirm: async () => {
                try {
                    Logger.info("Deleting StatefulSet", { namespace, name });
                    await DeleteStatefulSet(currentContext, namespace, name);
                    Logger.info("StatefulSet deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete StatefulSet", err);
                    alert(`Failed to delete statefulset: ${err.message || err}`);
                }
            }
        });
    };

    const handleViewLogs = async (statefulSet) => {
        Logger.info("View logs for StatefulSet", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name });
        const namespace = statefulSet.metadata.namespace;

        try {
            const allPods = await ListPods(namespace);

            // Filter pods that belong to this statefulset
            const statefulSetPods = allPods.filter(pod => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'StatefulSet' &&
                    ref.name === statefulSet.metadata.name
                );
            });

            if (statefulSetPods.length === 0) {
                Logger.info("No pods found for StatefulSet", { namespace, name: statefulSet.metadata.name });
                alert(`No pods found for statefulset "${statefulSet.metadata.name}".`);
                return;
            }

            const pod = statefulSetPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            // Build container map for all pods
            const podContainerMap = {};
            for (const p of statefulSetPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            Logger.info("Opening logs for StatefulSet pod", {
                namespace,
                statefulSet: statefulSet.metadata.name,
                pod: pod.metadata.name,
                totalPods: statefulSetPods.length
            });

            const tabId = `logs-statefulset-${statefulSet.metadata.name}`;
            openTab({
                id: tabId,
                title: `Logs: ${statefulSet.metadata.name}`,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={statefulSetPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={statefulSet.metadata.name}
                    />
                )
            });
        } catch (err) {
            Logger.error("Failed to get pods for StatefulSet", err);
            alert(`Failed to get pods for statefulset: ${err.message || err}`);
        }
    };

    return {
        handleEditYaml,
        handleRestart,
        handleDelete,
        handleViewLogs
    };
};
