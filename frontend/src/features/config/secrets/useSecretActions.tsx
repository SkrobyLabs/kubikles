import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { DeleteSecret } from 'wailsjs/go/main/App';
import SecretEditor from '~/components/shared/SecretEditor';
import { LazyDependencyGraph as DependencyGraph } from '~/components/lazy';
import Logger from '~/utils/Logger';
import { LockClosedIcon, PencilSquareIcon, ShareIcon } from '@heroicons/react/24/outline';
import { K8sSecret } from '~/types/k8s';
import { BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';

export interface SecretActionsReturn {
    handleEditYaml: (secret: K8sSecret) => void;
    handleEditKeyValue: (secret: K8sSecret) => void;
    handleShowDependencies: (secret: K8sSecret) => void;
    handleDelete: (secret: K8sSecret) => void;
}

export const useSecretActions = (): any => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (secret: K8sSecret): void => {
        Logger.info("Opening secret editor", { namespace: secret.metadata.namespace, secret: secret.metadata.name }, 'config');
        const tabId = `secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${secret.metadata.name}`,
            icon: LockClosedIcon,
            actionLabel: 'Edit',
            content: (
                <SecretEditor
                    namespace={secret.metadata.namespace!}
                    resourceName={secret.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'Secret', name: secret.metadata.name, namespace: secret.metadata.namespace },
        });
    };

    const handleEditKeyValue = (secret: K8sSecret): void => {
        Logger.info("Opening secret editor (key-value)", { namespace: secret.metadata.namespace, secret: secret.metadata.name }, 'config');
        const tabId = `secret-${secret.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${secret.metadata.name}`,
            icon: LockClosedIcon,
            actionLabel: 'Edit',
            content: (
                <SecretEditor
                    namespace={secret.metadata.namespace!}
                    resourceName={secret.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                    initialMode="keyvalue"
                />
            ),
            resourceMeta: { kind: 'Secret', name: secret.metadata.name, namespace: secret.metadata.namespace },
        });
    };

    const handleShowDependencies = (secret: K8sSecret): void => {
        Logger.info("Opening dependency graph", { namespace: secret.metadata.namespace, secret: secret.metadata.name }, 'config');
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
            ),
            resourceMeta: { kind: 'Secret', name: secret.metadata.name, namespace: secret.metadata.namespace },
        });
    };

    const handleDelete = (secret: K8sSecret): void => {
        const name = secret.metadata.name;
        const namespace = secret.metadata.namespace;
        Logger.info("Delete Secret requested", { namespace, name }, 'config');

        openModal({
            title: `Delete Secret ${name}?`,
            content: `Are you sure you want to delete secret "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteSecret(namespace, name);
                    Logger.info("Secret deleted successfully", { namespace, name }, 'config');
                    closeModal();
                } catch (err: any) {
                    Logger.error("Failed to delete secret", err, 'config');
                    addNotification({ type: 'error', title: 'Failed to delete secret', message: String(err.message || err) });
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
