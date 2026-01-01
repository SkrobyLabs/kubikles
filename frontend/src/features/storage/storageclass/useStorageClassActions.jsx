import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteStorageClass } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import StorageClassDetails from '../../../components/shared/StorageClassDetails';
import Logger from '../../../utils/Logger';

export const useStorageClassActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleShowDetails = (storageClass) => {
        Logger.info("Opening StorageClass details", { name: storageClass.metadata.name });
        const tabId = `details-storageclass-${storageClass.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${storageClass.metadata.name}`,
            content: (
                <StorageClassDetails
                    storageClass={storageClass}
                    tabContext={currentContext}
                />
            )
        });
    };

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
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleDelete = (storageClass) => {
        const name = storageClass.metadata.name;
        Logger.info("Delete StorageClass requested", { name });

        openModal({
            title: `Delete StorageClass ${name}?`,
            content: `Are you sure you want to delete StorageClass "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteStorageClass(name);
                    Logger.info("StorageClass deleted successfully", { name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete StorageClass", err);
                    alert(`Failed to delete StorageClass: ${err}`);
                }
            }
        });
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
