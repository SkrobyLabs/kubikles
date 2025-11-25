import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeletePod, ForceDeletePod, OpenTerminal } from '../../../../wailsjs/go/main/App';
import LogViewer from '../../../components/shared/LogViewer';
import Terminal from '../../../components/shared/Terminal';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const usePodActions = () => {
    const { openTab, closeTab } = useUI();
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

    const handleDelete = async (namespace, name, isTerminating = false) => {
        const actionType = isTerminating ? 'Force Delete' : 'Delete';
        Logger.info(`Action: ${actionType} Pod`, { namespace, name, context: currentContext });

        if (!confirm(`Are you sure you want to ${actionType.toLowerCase()} pod ${name}?`)) {
            Logger.info("Delete action cancelled by user");
            return;
        }

        try {
            if (isTerminating) {
                await ForceDeletePod(currentContext, namespace, name);
            } else {
                await DeletePod(currentContext, namespace, name);
            }
            Logger.info(`Pod ${actionType.toLowerCase()}d successfully`, { namespace, name });
        } catch (err) {
            Logger.error(`Failed to ${actionType.toLowerCase()} pod`, err);
            alert(`Failed to ${actionType.toLowerCase()} pod: ${err}`);
        }
    };

    return {
        openLogs,
        handleShell,
        handleEditYaml,
        handleDelete
    };
};
