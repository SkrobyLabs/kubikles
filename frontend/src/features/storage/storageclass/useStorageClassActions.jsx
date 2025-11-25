import React from 'react';
import { useUI } from '../../../context/UIContext';
import { DeleteStorageClass } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useStorageClassActions = () => {
    const { openTab, closeTab } = useUI();

    const handleEditYaml = (storageClass) => {
        Logger.info("Opening StorageClass YAML editor", { name: storageClass.metadata.name });
        const tabId = `storageclass-${storageClass.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${storageClass.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="storageclass"
                    resourceName={storageClass.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = async (storageClass) => {
        if (!confirm(`Are you sure you want to delete StorageClass ${storageClass.metadata.name}?`)) return;

        Logger.info("Deleting StorageClass", { name: storageClass.metadata.name });
        try {
            await DeleteStorageClass(storageClass.metadata.name);
            Logger.info("Delete triggered successfully", { name: storageClass.metadata.name });
        } catch (err) {
            Logger.error("Failed to delete StorageClass", err);
            alert(`Failed to delete StorageClass: ${err}`);
        }
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
