import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteSecret } from '../../../../wailsjs/go/main/App';
import SecretEditor from '../../../components/shared/SecretEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import Logger from '../../../utils/Logger';

export const useSecretActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (secret) => {
        Logger.info("Opening secret editor", { namespace: secret.metadata.namespace, secret: secret.metadata.name });
        const tabId = `secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${secret.metadata.name}`,
            content: (
                <SecretEditor
                    namespace={secret.metadata.namespace}
                    resourceName={secret.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleShowDependencies = (secret) => {
        Logger.info("Opening dependency graph", { namespace: secret.metadata.namespace, secret: secret.metadata.name });
        const tabId = `deps-secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${secret.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="secret"
                    namespace={secret.metadata.namespace}
                    resourceName={secret.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = (secret) => {
        const name = secret.metadata.name;
        const namespace = secret.metadata.namespace;
        Logger.info("Delete Secret requested", { namespace, name });

        openModal({
            title: `Delete Secret ${name}?`,
            content: `Are you sure you want to delete secret "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteSecret(namespace, name);
                    Logger.info("Secret deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete secret", err);
                    alert(`Failed to delete secret: ${err}`);
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
