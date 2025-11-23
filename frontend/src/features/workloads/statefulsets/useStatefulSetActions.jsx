import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteStatefulSet, RestartStatefulSet, LogDebug } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';

export const useStatefulSetActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (statefulSet) => {
        const tabId = `yaml-statefulset-${statefulSet.metadata.uid}`;
        openTab({
            id: tabId,
            title: `YAML: ${statefulSet.metadata.name}`,
            content: (
                <YamlEditor
                    namespace={statefulSet.metadata.namespace}
                    podName={statefulSet.metadata.name}
                    isStatefulSet={true}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleRestart = async (statefulSet) => {
        const msg = `Restarting statefulset: ${statefulSet.metadata.name}`;
        console.log(msg);
        try {
            await LogDebug(msg);
            await RestartStatefulSet(currentContext, statefulSet.metadata.namespace, statefulSet.metadata.name);
            console.log("Restart triggered successfully");
        } catch (err) {
            console.error("Failed to restart statefulset", err);
            alert(`Failed to restart statefulset: ${err}`);
        }
    };

    const handleDelete = async (statefulSet) => {
        if (!confirm(`Are you sure you want to delete statefulset ${statefulSet.metadata.name}?`)) return;

        const msg = `Deleting statefulset: ${statefulSet.metadata.name}`;
        console.log(msg);
        try {
            await LogDebug(msg);
            await DeleteStatefulSet(currentContext, statefulSet.metadata.namespace, statefulSet.metadata.name);
            console.log("Delete triggered successfully");
        } catch (err) {
            console.error("Failed to delete statefulset", err);
            alert(`Failed to delete statefulset: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleRestart,
        handleDelete
    };
};
