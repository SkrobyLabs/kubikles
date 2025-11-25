import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteDaemonSet, RestartDaemonSet, ListPods } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import LogViewer from '../../../components/shared/LogViewer';
import Logger from '../../../utils/Logger';

export const useDaemonSetActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (daemonSet) => {
        Logger.info("Opening YAML editor for DaemonSet", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name });
        const tabId = `edit-ds-${daemonSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${daemonSet.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="daemonset"
                    namespace={daemonSet.metadata.namespace}
                    resourceName={daemonSet.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleRestart = async (daemonSet) => {
        Logger.info("Restart DaemonSet requested", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name });
        try {
            await RestartDaemonSet(currentContext, daemonSet.metadata.namespace, daemonSet.metadata.name);
            Logger.info("DaemonSet restarted successfully", { name: daemonSet.metadata.name });
        } catch (err) {
            Logger.error("Failed to restart DaemonSet", err);
            alert(`Failed to restart daemonset: ${err.message || err}`);
        }
    };

    const handleDelete = async (daemonSet) => {
        Logger.info("Delete DaemonSet requested", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name });
        const name = daemonSet.metadata.name;
        const namespace = daemonSet.metadata.namespace;

        openModal({
            title: `Delete DaemonSet ${name}?`,
            content: `Are you sure you want to delete daemonset "${name}"? This will also delete all associated pods.`,
            onConfirm: async () => {
                try {
                    Logger.info("Deleting DaemonSet", { namespace, name });
                    await DeleteDaemonSet(currentContext, namespace, name);
                    Logger.info("DaemonSet deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete DaemonSet", err);
                    alert(`Failed to delete daemonset: ${err.message || err}`);
                }
            }
        });
    };

    const handleViewLogs = async (daemonSet) => {
        Logger.info("View logs for DaemonSet", { namespace: daemonSet.metadata.namespace, name: daemonSet.metadata.name });
        const namespace = daemonSet.metadata.namespace;

        try {
            const allPods = await ListPods(namespace);

            // Filter pods that belong to this daemonset
            const daemonSetPods = allPods.filter(pod => {
                const ownerRefs = pod.metadata?.ownerReferences || [];
                return ownerRefs.some(ref =>
                    ref.kind === 'DaemonSet' &&
                    ref.name === daemonSet.metadata.name
                );
            });

            if (daemonSetPods.length === 0) {
                Logger.info("No pods found for DaemonSet", { namespace, name: daemonSet.metadata.name });
                alert(`No pods found for daemonset "${daemonSet.metadata.name}".`);
                return;
            }

            const pod = daemonSetPods[0];
            const containers = [
                ...(pod.spec?.initContainers || []).map(c => c.name),
                ...(pod.spec?.containers || []).map(c => c.name)
            ];

            // Build container map for all pods
            const podContainerMap = {};
            for (const p of daemonSetPods) {
                podContainerMap[p.metadata.name] = [
                    ...(p.spec?.initContainers || []).map(c => c.name),
                    ...(p.spec?.containers || []).map(c => c.name)
                ];
            }

            Logger.info("Opening logs for DaemonSet pod", {
                namespace,
                daemonSet: daemonSet.metadata.name,
                pod: pod.metadata.name,
                totalPods: daemonSetPods.length
            });

            const tabId = `logs-daemonset-${daemonSet.metadata.name}`;
            openTab({
                id: tabId,
                title: `Logs: ${daemonSet.metadata.name}`,
                content: (
                    <LogViewer
                        namespace={namespace}
                        pod={pod.metadata.name}
                        containers={containers}
                        siblingPods={daemonSetPods.map(p => p.metadata.name)}
                        podContainerMap={podContainerMap}
                        ownerName={daemonSet.metadata.name}
                    />
                )
            });
        } catch (err) {
            Logger.error("Failed to get pods for DaemonSet", err);
            alert(`Failed to get pods for daemonset: ${err.message || err}`);
        }
    };

    return {
        handleEditYaml,
        handleRestart,
        handleDelete,
        handleViewLogs
    };
};
