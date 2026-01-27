import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { useNotification } from '../../../context/NotificationContext';
import { DeleteSecret } from '../../../../wailsjs/go/main/App';
import SecretEditor from '../../../components/shared/SecretEditor';
import { LazyDependencyGraph as DependencyGraph } from '../../../components/lazy';
import Logger from '../../../utils/Logger';
import { LockClosedIcon, PencilSquareIcon, ShareIcon } from '@heroicons/react/24/outline';

export const useSecretActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (secret) => {
        Logger.info("Opening secret editor", { namespace: secret.metadata.namespace, secret: secret.metadata.name });
        const tabId = `secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${secret.metadata.name}`,
            icon: LockClosedIcon,
            actionLabel: 'Edit',
            content: (
                <SecretEditor
                    namespace={secret.metadata.namespace}
                    resourceName={secret.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleEditKeyValue = (secret) => {
        Logger.info("Opening secret editor (key-value)", { namespace: secret.metadata.namespace, secret: secret.metadata.name });
        const tabId = `secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${secret.metadata.name}`,
            icon: LockClosedIcon,
            actionLabel: 'Edit',
            content: (
                <SecretEditor
                    namespace={secret.metadata.namespace}
                    resourceName={secret.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                    initialMode="keyvalue"
                />
            )
        });
    };

    const handleShowDependencies = (secret) => {
        Logger.info("Opening dependency graph", { namespace: secret.metadata.namespace, secret: secret.metadata.name });
        const tabId = `deps-secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${secret.metadata.name}`,
            icon: LockClosedIcon,
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
                    addNotification({ type: 'error', title: 'Failed to delete secret', message: String(err) });
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleEditKeyValue,
        handleShowDependencies,
        handleDelete
    };
};
