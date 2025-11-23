import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeletePod, ForceDeletePod, OpenTerminal, LogDebug } from '../../../../wailsjs/go/main/App';
import LogViewer from '../../../components/shared/LogViewer';
import Terminal from '../../../components/shared/Terminal';
import YamlEditor from '../../../components/shared/YamlEditor';

export const usePodActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const openLogs = (namespace, podName, containers = []) => {
        const tabId = `logs-${podName}`;
        openTab({
            id: tabId,
            title: `Logs: ${podName}`,
            content: <LogViewer namespace={namespace} pod={podName} containers={containers} />
        });
    };

    const handleShell = async (namespace, podName) => {
        try {
            const url = await OpenTerminal(currentContext, namespace, podName, "");
            const tabId = `shell-${podName}`;
            openTab({
                id: tabId,
                title: `Shell: ${podName}`,
                content: <Terminal url={url} />
            });
        } catch (err) {
            console.error("Failed to open shell", err);
            alert("Failed to open shell: " + err);
        }
    };

    const handleEditYaml = (pod) => {
        const tabId = `yaml-${pod.metadata.uid}`;
        openTab({
            id: tabId,
            title: `YAML: ${pod.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={pod.metadata.namespace}
                    podName={pod.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = async (namespace, name, isTerminating = false) => {
        const actionType = isTerminating ? 'Force Delete' : 'Delete';
        const msg = `handleDeletePod(${actionType}) called for: ${name}, Namespace: ${namespace}, Context: ${currentContext}`;
        console.log(msg);
        try {
            await LogDebug(msg);
        } catch (e) {
            console.error("Failed to LogDebug", e);
        }

        console.log("Auto-confirmed delete for debugging");

        try {
            console.log("Calling backend DeletePod...");
            if (isTerminating) {
                await ForceDeletePod(currentContext, namespace, name);
            } else {
                await DeletePod(currentContext, namespace, name);
            }
            console.log("Backend DeletePod returned success");
        } catch (err) {
            const action = isTerminating ? 'force delete' : 'delete';
            const errMsg = `Failed to ${action} pod: ${err}`;
            console.error(errMsg);
            await LogDebug(errMsg);
            alert(errMsg);
        }
    };

    return {
        openLogs,
        handleShell,
        handleEditYaml,
        handleDelete
    };
};
