import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteStatefulSet, RestartStatefulSet } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useStatefulSetActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (statefulSet) => {
        Logger.info("Opening YAML editor", { namespace: statefulSet.metadata.namespace, statefulSet: statefulSet.metadata.name });
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
        if (!confirm(`Are you sure you want to delete statefulset ${statefulSet.metadata.name}?`)) return;

        Logger.info("Deleting statefulset", { namespace: statefulSet.metadata.namespace, name: statefulSet.metadata.name });
        try {
            await DeleteStatefulSet(currentContext, statefulSet.metadata.namespace, statefulSet.metadata.name);
            Logger.info("Delete triggered successfully", { name: statefulSet.metadata.name });
        } catch (err) {
            Logger.error("Failed to delete statefulset", err);
            alert(`Failed to delete statefulset: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleRestart,
        handleDelete
    };
};
