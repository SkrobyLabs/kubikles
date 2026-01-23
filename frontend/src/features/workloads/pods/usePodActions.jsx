import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeletePod, ForceDeletePod } from '../../../../wailsjs/go/main/App';
import LogViewer from '../../../components/shared/log-viewer';
import { LazyTerminal as Terminal, LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph } from '../../../components/lazy';
import PodDetails from '../../../components/shared/PodDetails';
import Logger from '../../../utils/Logger';
import { CubeIcon } from '@heroicons/react/24/outline';

export const usePodActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const openLogs = (namespace, podName, containers = [], siblingPods = [], podContainerMap = {}, ownerName = '', podCreationTime = '') => {
        Logger.info("Opening logs", { namespace, pod: podName });
        const tabId = `logs-pod-${podName}`;
        openTab({
            id: tabId,
            title: podName,
            icon: CubeIcon,
            actionLabel: 'Logs',
            keepAlive: true,
            content: <LogViewer namespace={namespace} pod={podName} containers={containers} siblingPods={siblingPods} podContainerMap={podContainerMap} ownerName={ownerName} podCreationTime={podCreationTime} tabContext={currentContext} />
        });
    };

    const handleShell = (namespace, podName) => {
        Logger.info("Opening shell", { namespace, pod: podName });
        const tabId = `terminal-pod-${podName}`;
        openTab({
            id: tabId,
            title: podName,
            icon: CubeIcon,
            actionLabel: 'Shell',
            keepAlive: true,
            content: (
                <Terminal
                    namespace={namespace}
                    pod={podName}
                    container=""
                    context={currentContext}
                />
            )
        });
        Logger.info("Shell opened successfully", { namespace, pod: podName });
    };

    const handleEditYaml = (pod) => {
        Logger.info("Opening YAML editor", { namespace: pod.metadata.namespace, pod: pod.metadata.name });
        const tabId = `yaml-pod-${pod.metadata.uid}`;
        openTab({
            id: tabId,
            title: pod.metadata.name,
            icon: CubeIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="pod"
                    namespace={pod.metadata.namespace}
                    resourceName={pod.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (pod) => {
        Logger.info("Opening dependency graph", { namespace: pod.metadata.namespace, pod: pod.metadata.name });
        const tabId = `deps-pod-${pod.metadata.uid}`;
        openTab({
            id: tabId,
            title: pod.metadata.name,
            icon: CubeIcon,
            actionLabel: 'Deps',
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

    const handleShowDetails = (pod) => {
        Logger.info("Opening pod details", { namespace: pod.metadata.namespace, pod: pod.metadata.name });
        const tabId = `details-pod-${pod.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${pod.metadata.name}`,
            icon: CubeIcon,
            content: (
                <PodDetails
                    pod={pod}
                    tabContext={currentContext}
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
        handleShowDetails,
        handleDelete
    };
};
