import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteServiceAccount } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import Logger from '../../../utils/Logger';

export const useServiceAccountActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (serviceAccount) => {
        Logger.info("Opening ServiceAccount editor", { namespace: serviceAccount.metadata.namespace, name: serviceAccount.metadata.name });
        const tabId = `serviceaccount-${serviceAccount.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${serviceAccount.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="serviceaccount"
                    namespace={serviceAccount.metadata.namespace}
                    resourceName={serviceAccount.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleDelete = (serviceAccount) => {
        const name = serviceAccount.metadata.name;
        const namespace = serviceAccount.metadata.namespace;
        Logger.info("Delete ServiceAccount requested", { namespace, name });

        openModal({
            title: `Delete ServiceAccount ${name}?`,
            content: `Are you sure you want to delete service account "${name}" in namespace "${namespace}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteServiceAccount(currentContext, namespace, name);
                    Logger.info("ServiceAccount deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete ServiceAccount", err);
                    alert(`Failed to delete service account: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
