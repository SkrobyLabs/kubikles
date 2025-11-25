import React from 'react';
import { useUI } from '../../../context/UIContext';
import { DeletePV } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const usePVActions = () => {
    const { openTab, closeTab } = useUI();

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
                />
            )
        });
    };

    const handleDelete = async (pv) => {
        if (!confirm(`Are you sure you want to delete PV ${pv.metadata.name}?`)) return;

        Logger.info("Deleting PV", { name: pv.metadata.name });
        try {
            await DeletePV(pv.metadata.name);
            Logger.info("Delete triggered successfully", { name: pv.metadata.name });
        } catch (err) {
            Logger.error("Failed to delete PV", err);
            alert(`Failed to delete PV: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
