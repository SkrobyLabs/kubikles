import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeletePod, ForceDeletePod, OpenTerminal } from '../../../../wailsjs/go/main/App';
import LogViewer from '../../../components/shared/LogViewer';
import Terminal from '../../../components/shared/Terminal';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import Logger from '../../../utils/Logger';

export const usePodActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const openLogs = (namespace, podName, containers = [], siblingPods = [], podContainerMap = {}, ownerName = '') => {
        Logger.info("Opening logs", { namespace, pod: podName });
        const tabId = `logs-${podName}`;
        openTab({
            id: tabId,
            title: `Logs: ${podName}`,
            content: <LogViewer namespace={namespace} pod={podName} containers={containers} siblingPods={siblingPods} podContainerMap={podContainerMap} ownerName={ownerName} />
        });
    };

    const handleShell = async (namespace, podName) => {
        Logger.info("Opening shell", { namespace, pod: podName });
        try {
            const url = await OpenTerminal(currentContext, namespace, podName, "");
            const tabId = `shell-${podName}`;
            openTab({
                id: tabId,
                title: `Shell: ${podName}`,
                content: <Terminal url={url} />
            });
            Logger.info("Shell opened successfully", { namespace, pod: podName });
        } catch (err) {
            Logger.error("Failed to open shell", err);
            alert("Failed to open shell: " + err);
        }
    };

    const handleEditYaml = (pod) => {
        Logger.info("Opening YAML editor", { namespace: pod.metadata.namespace, pod: pod.metadata.name });
        const tabId = `yaml-${pod.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${pod.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="pod"
                    namespace={pod.metadata.namespace}
                    resourceName={pod.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleShowDependencies = (pod) => {
        Logger.info("Opening dependency graph", { namespace: pod.metadata.namespace, pod: pod.metadata.name });
        const tabId = `deps-${pod.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${pod.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="pod"
                    namespace={pod.metadata.namespace}
                    resourceName={pod.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = (namespace, name, isTerminating = false) => {
        const actionType = isTerminating ? 'Force Delete' : 'Delete';
        Logger.info(`Action: ${actionType} Pod`, { namespace, name, context: currentContext });

        openModal({
            title: `${actionType} Pod ${name}?`,
            content: isTerminating
                ? `Are you sure you want to force delete pod "${name}"? This will immediately remove the pod without waiting for graceful termination.`
                : `Are you sure you want to delete pod "${name}"?`,
            confirmText: actionType,
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    if (isTerminating) {
                        await ForceDeletePod(currentContext, namespace, name);
                    } else {
                        await DeletePod(currentContext, namespace, name);
                    }
                    Logger.info(`Pod ${actionType.toLowerCase()}d successfully`, { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error(`Failed to ${actionType.toLowerCase()} pod`, err);
                    alert(`Failed to ${actionType.toLowerCase()} pod: ${err}`);
                }
            }
        });
    };

    return {
        openLogs,
        handleShell,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
