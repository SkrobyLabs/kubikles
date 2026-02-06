import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { DeleteConfigMap } from 'wailsjs/go/main/App';
import ConfigMapEditor from '~/components/shared/ConfigMapEditor';
import DependencyGraph from '~/components/shared/DependencyGraph';
import Logger from '~/utils/Logger';
import { DocumentTextIcon, PencilSquareIcon, ShareIcon } from '@heroicons/react/24/outline';
import { K8sConfigMap } from '~/types/k8s';

export interface ConfigMapActionsReturn {
    handleEditYaml: (configMap: K8sConfigMap) => void;
    handleEditKeyValue: (configMap: K8sConfigMap) => void;
    handleShowDependencies: (configMap: K8sConfigMap) => void;
    handleDelete: (configMap: K8sConfigMap) => void;
}

export const useConfigMapActions = (): any => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (configMap: K8sConfigMap): void => {
        Logger.info("Opening configmap editor", { namespace: configMap.metadata.namespace, configMap: configMap.metadata.name });
        const tabId = `configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${configMap.metadata.name}`,
            icon: DocumentTextIcon,
            actionLabel: 'Edit',
            content: (
                <ConfigMapEditor
                    namespace={configMap.metadata.namespace}
                    resourceName={configMap.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'ConfigMap', name: configMap.metadata.name, namespace: configMap.metadata.namespace },
        });
    };

    const handleEditKeyValue = (configMap: K8sConfigMap): void => {
        Logger.info("Opening configmap editor (key-value)", { namespace: configMap.metadata.namespace, configMap: configMap.metadata.name });
        const tabId = `configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${configMap.metadata.name}`,
            icon: DocumentTextIcon,
            actionLabel: 'Edit',
            content: (
                <ConfigMapEditor
                    namespace={configMap.metadata.namespace}
                    resourceName={configMap.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                    initialMode="keyvalue"
                />
            ),
            resourceMeta: { kind: 'ConfigMap', name: configMap.metadata.name, namespace: configMap.metadata.namespace },
        });
    };

    const handleShowDependencies = (configMap: K8sConfigMap): void => {
        Logger.info("Opening dependency graph", { namespace: configMap.metadata.namespace, configMap: configMap.metadata.name });
        const tabId = `deps-configmap-${configMap.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${configMap.metadata.name}`,
            icon: DocumentTextIcon,
            content: (
                <DependencyGraph
                    resourceType="configmap"
                    namespace={configMap.metadata.namespace}
                    resourceName={configMap.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            ),
            resourceMeta: { kind: 'ConfigMap', name: configMap.metadata.name, namespace: configMap.metadata.namespace },
        });
    };

    const handleDelete = (configMap: K8sConfigMap): void => {
        const name = configMap.metadata.name;
        const namespace = configMap.metadata.namespace;
        Logger.info("Delete ConfigMap requested", { namespace, name });

        openModal({
            title: `Delete ConfigMap ${name}?`,
            content: `Are you sure you want to delete configmap "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await (DeleteConfigMap as (namespace: string, name: string) => Promise<void>)(namespace!, name);
                    Logger.info("ConfigMap deleted successfully", { namespace, name });
                    closeModal();
                } catch (err: unknown) {
                    Logger.error("Failed to delete configmap", err);
                    addNotification({ type: 'error', title: 'Failed to delete configmap', message: String(err) });
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
