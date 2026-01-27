import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { useNotification } from '../../../context/NotificationContext';
import { DeleteConfigMap } from '../../../../wailsjs/go/main/App';
import ConfigMapEditor from '../../../components/shared/ConfigMapEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import Logger from '../../../utils/Logger';
import { DocumentTextIcon, PencilSquareIcon, ShareIcon } from '@heroicons/react/24/outline';

export const useConfigMapActions = () => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (configMap) => {
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
            )
        });
    };

    const handleEditKeyValue = (configMap) => {
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
            )
        });
    };

    const handleShowDependencies = (configMap) => {
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
            )
        });
    };

    const handleDelete = (configMap) => {
        const name = configMap.metadata.name;
        const namespace = configMap.metadata.namespace;
        Logger.info("Delete ConfigMap requested", { namespace, name });

        openModal({
            title: `Delete ConfigMap ${name}?`,
            content: `Are you sure you want to delete configmap "${name}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteConfigMap(namespace, name);
                    Logger.info("ConfigMap deleted successfully", { namespace, name });
                    closeModal();
                } catch (err) {
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
