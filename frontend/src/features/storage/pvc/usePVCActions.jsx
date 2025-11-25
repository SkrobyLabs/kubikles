import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeletePVC } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import Logger from '../../../utils/Logger';

export const usePVCActions = () => {
    const { openTab, closeTab } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (pvc) => {
        Logger.info("Opening PVC YAML editor", { namespace: pvc.metadata.namespace, name: pvc.metadata.name });
        const tabId = `pvc-${pvc.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${pvc.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="pvc"
                    namespace={pvc.metadata.namespace}
                    resourceName={pvc.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleShowDependencies = (pvc) => {
        Logger.info("Opening PVC dependency graph", { namespace: pvc.metadata.namespace, name: pvc.metadata.name });
        const tabId = `deps-pvc-${pvc.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${pvc.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="pvc"
                    namespace={pvc.metadata.namespace}
                    resourceName={pvc.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = async (pvc) => {
        if (!confirm(`Are you sure you want to delete PVC ${pvc.metadata.name}?`)) return;

        Logger.info("Deleting PVC", { namespace: pvc.metadata.namespace, name: pvc.metadata.name });
        try {
            await DeletePVC(pvc.metadata.namespace, pvc.metadata.name);
            Logger.info("Delete triggered successfully", { name: pvc.metadata.name });
        } catch (err) {
            Logger.error("Failed to delete PVC", err);
            alert(`Failed to delete PVC: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
