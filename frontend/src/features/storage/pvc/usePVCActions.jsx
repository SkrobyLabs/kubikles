import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeletePVC } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import PVCDetails from '../../../components/shared/PVCDetails';
import Logger from '../../../utils/Logger';

export const usePVCActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleShowDetails = (pvc) => {
        Logger.info("Opening PVC details", { namespace: pvc.metadata.namespace, name: pvc.metadata.name });
        const tabId = `details-pvc-${pvc.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${pvc.metadata.name}`,
            content: (
                <PVCDetails
                    pvc={pvc}
                    tabContext={currentContext}
                />
            )
        });
    };

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
                    tabContext={currentContext}
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

    const handleDelete = (pvc) => {
        const name = pvc.metadata.name;
        const namespace = pvc.metadata.namespace;
        Logger.info("Delete PVC requested", { namespace, name });

        openModal({
            title: `Delete PVC ${name}?`,
            content: `Are you sure you want to delete PVC "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    Logger.info("Deleting PVC", { namespace, name });
                    await DeletePVC(namespace, name);
                    Logger.info("PVC deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete PVC", err);
                    alert(`Failed to delete PVC: ${err}`);
                }
            }
        });
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
