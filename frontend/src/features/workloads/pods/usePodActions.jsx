import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { useNotification } from '../../../context/NotificationContext';
import { DeletePod, ForceDeletePod } from '../../../../wailsjs/go/main/App';
import LogViewer from '../../../components/shared/log-viewer';
import { LazyYamlEditor as YamlEditor, LazyDependencyGraph as DependencyGraph, LazyPodFileBrowser as PodFileBrowser } from '../../../components/lazy';
import PodShellTab from './PodShellTab';
import PodDetails from '../../../components/shared/PodDetails';
import Logger from '../../../utils/Logger';
import { CubeIcon } from '@heroicons/react/24/outline';

export const usePodActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const openLogs = (namespace, podName, containers = [], siblingPods = [], podContainerMap = {}, ownerName = '', podCreationTime = '') => {
        Logger.info("Opening logs", { namespace, pod: podName });
        const tabId = `logs-pod-${podName}`;
        openTab({
            id: tabId,
            title: podName,
            icon: CubeIcon,
            actionLabel: 'Logs',
            keepAlive: true,
            content: <LogViewer namespace={namespace} pod={podName} containers={containers} siblingPods={siblingPods} podContainerMap={podContainerMap} ownerName={ownerName} podCreationTime={podCreationTime} tabContext={currentContext} />,
            resourceMeta: { kind: 'Pod', name: podName, namespace },
        });
    };

    // Helper to build container objects with status
    const getContainersWithStatus = (pod) => {
        const initStatuses = pod.status?.initContainerStatuses || [];
        const containerStatuses = pod.status?.containerStatuses || [];

        // Build containers with status info
        const containers = [];

        // Add init containers
        for (const spec of (pod.spec?.initContainers || [])) {
            const status = initStatuses.find(s => s.name === spec.name);
            if (status?.state?.running || status?.state?.waiting) {
                containers.push({ name: spec.name, status, isInit: true });
            }
        }

        // Add regular containers
        for (const spec of (pod.spec?.containers || [])) {
            const status = containerStatuses.find(s => s.name === spec.name);
            if (status?.state?.running || status?.state?.waiting) {
                containers.push({ name: spec.name, status, isInit: false });
            }
        }

        // Fallback: if no running/waiting containers, use spec names without status
        if (containers.length === 0) {
            for (const spec of (pod.spec?.initContainers || [])) {
                containers.push({ name: spec.name, status: null, isInit: true });
            }
            for (const spec of (pod.spec?.containers || [])) {
                containers.push({ name: spec.name, status: null, isInit: false });
            }
        }

        return containers;
    };

    const handleShell = (pod) => {
        const namespace = pod.metadata?.namespace;
        const podName = pod.metadata?.name;
        Logger.info("Opening shell", { namespace, pod: podName });

        const containers = getContainersWithStatus(pod);

        const tabId = `terminal-pod-${podName}`;
        openTab({
            id: tabId,
            title: podName,
            icon: CubeIcon,
            actionLabel: 'Shell',
            keepAlive: true,
            content: (
                <PodShellTab
                    namespace={namespace}
                    pod={podName}
                    containers={containers}
                    context={currentContext}
                />
            ),
            resourceMeta: { kind: 'Pod', name: podName, namespace },
        });
        Logger.info("Shell opened successfully", { namespace, pod: podName });
    };

    const handleFiles = (pod) => {
        const namespace = pod.metadata?.namespace;
        const podName = pod.metadata?.name;
        Logger.info("Opening file browser", { namespace, pod: podName });

        const containers = getContainersWithStatus(pod);

        // Always open tab - PodFileBrowser handles container selection inline
        const tabId = `files-${podName}`;
        openTab({
            id: tabId,
            title: podName,
            icon: CubeIcon,
            actionLabel: 'Files',
            keepAlive: true,
            content: (
                <PodFileBrowser
                    namespace={namespace}
                    pod={podName}
                    containers={containers}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'Pod', name: podName, namespace },
        });
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
            ),
            resourceMeta: { kind: 'Pod', name: pod.metadata.name, namespace: pod.metadata.namespace },
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
            ),
            resourceMeta: { kind: 'Pod', name: pod.metadata.name, namespace: pod.metadata.namespace },
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
            ),
            resourceMeta: { kind: 'Pod', name: pod.metadata.name, namespace: pod.metadata.namespace },
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
                        await ForceDeletePod(namespace, name);
                    } else {
                        await DeletePod(namespace, name);
                    }
                    Logger.info(`Pod ${actionType.toLowerCase()}d successfully`, { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error(`Failed to ${actionType.toLowerCase()} pod`, err);
                    addNotification({ type: 'error', title: `Failed to ${actionType.toLowerCase()} pod`, message: String(err) });
                }
            }
        });
    };

    return {
        openLogs,
        handleShell,
        handleFiles,
        handleEditYaml,
        handleShowDependencies,
        handleShowDetails,
        handleDelete
    };
};
