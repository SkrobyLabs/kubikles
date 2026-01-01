import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeletePV } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import PVDetails from '../../../components/shared/PVDetails';
import Logger from '../../../utils/Logger';

export const usePVActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleShowDetails = (pv) => {
        Logger.info("Opening PV details", { name: pv.metadata.name });
        const tabId = `details-pv-${pv.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${pv.metadata.name}`,
            content: (
                <PVDetails
                    pv={pv}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleEditYaml = (pv) => {
        Logger.info("Opening PV YAML editor", { name: pv.metadata.name });
        const tabId = `pv-${pv.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${pv.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="pv"
                    resourceName={pv.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (pv) => {
        Logger.info("Opening PV dependency graph", { name: pv.metadata.name });
        const tabId = `deps-pv-${pv.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${pv.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="pv"
                    resourceName={pv.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = (pv) => {
        const name = pv.metadata.name;
        Logger.info("Delete PV requested", { name });

        openModal({
            title: `Delete PV ${name}?`,
            content: `Are you sure you want to delete PersistentVolume "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeletePV(name);
                    Logger.info("PV deleted successfully", { name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete PV", err);
                    alert(`Failed to delete PV: ${err}`);
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
