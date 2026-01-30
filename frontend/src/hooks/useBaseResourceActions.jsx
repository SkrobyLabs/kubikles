import React from 'react';
import { useUI } from '../context/UIContext';
import { useK8s } from '../context/K8sContext';
import { useNotification } from '../context/NotificationContext';
import { LazyYamlEditor, LazyDependencyGraph } from '../components/lazy';
import Logger from '../utils/Logger';
import { getResourceIcon } from '../utils/resourceIcons';

/**
 * Creates standard resource action handlers for showing details, editing YAML,
 * showing dependencies, and deleting resources.
 *
 * @param {Object} config - Configuration object
 * @param {string} config.resourceType - Resource type identifier (e.g., 'deployment', 'configmap')
 * @param {string} config.resourceLabel - Human-readable label (e.g., 'Deployment', 'ConfigMap')
 * @param {React.Component} config.DetailsComponent - Component to render for details view
 * @param {string} config.detailsPropName - Prop name for passing resource to DetailsComponent (e.g., 'deployment')
 * @param {boolean} [config.isNamespaced=true] - Whether the resource is namespaced
 * @param {boolean} [config.hasDependencies=true] - Whether to include dependency graph action
 * @param {Function} [config.getTabTitle] - Custom function to get tab title from resource (defaults to metadata.name)
 * @returns {Object} Object containing action handlers
 */
export function useBaseResourceActions(config) {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const {
        resourceType,
        resourceLabel,
        DetailsComponent,
        detailsPropName,
        isNamespaced = true,
        hasDependencies = true,
        getTabTitle = (resource) => resource.metadata?.name,
    } = config;

    /**
     * Opens a details tab for the resource
     */
    const handleShowDetails = (resource) => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace;
        Logger.info(`Opening ${resourceLabel} details`, { namespace, name });

        const tabId = `details-${resourceType}-${resource.metadata.uid}`;
        const props = {
            [detailsPropName]: resource,
            tabContext: currentContext,
        };

        openTab({
            id: tabId,
            title: getTabTitle(resource),
            icon: getResourceIcon(resourceType),
            content: <DetailsComponent {...props} />,
            resourceMeta: { kind: resourceLabel, name, namespace },
        });
    };

    /**
     * Opens a YAML editor tab for the resource
     */
    const handleEditYaml = (resource) => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace;
        Logger.info(`Opening ${resourceLabel} YAML editor`, { namespace, name });

        const tabId = `yaml-${resourceType}-${resource.metadata.uid}`;
        openTab({
            id: tabId,
            title: name,
            icon: getResourceIcon(resourceType),
            actionLabel: 'Edit',
            content: (
                <LazyYamlEditor
                    resourceType={resourceType}
                    namespace={isNamespaced ? namespace : undefined}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: resourceLabel, name, namespace },
        });
    };

    /**
     * Opens a dependency graph tab for the resource
     */
    const handleShowDependencies = hasDependencies ? (resource) => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace;
        Logger.info(`Opening ${resourceLabel} dependency graph`, { namespace, name });

        const tabId = `deps-${resourceType}-${resource.metadata.uid}`;
        openTab({
            id: tabId,
            title: name,
            icon: getResourceIcon(resourceType),
            actionLabel: 'Deps',
            content: (
                <LazyDependencyGraph
                    resourceType={resourceType}
                    namespace={isNamespaced ? namespace : undefined}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                />
            ),
            resourceMeta: { kind: resourceLabel, name, namespace },
        });
    } : undefined;

    /**
     * Creates a delete handler with confirmation modal
     * @param {Function} deleteFn - The delete function to call
     * @param {Object} [options] - Additional options
     * @param {string} [options.confirmMessage] - Custom confirmation message
     * @returns {Function} Delete handler
     */
    const createDeleteHandler = (deleteFn, options = {}) => {
        return (resource) => {
            const name = resource.metadata?.name;
            const namespace = resource.metadata?.namespace;
            Logger.info(`Delete ${resourceLabel} requested`, { namespace, name });

            const confirmMessage = options.confirmMessage ||
                `Are you sure you want to delete ${resourceLabel.toLowerCase()} "${name}"? This action cannot be undone.`;

            openModal({
                title: `Delete ${resourceLabel} ${name}?`,
                content: confirmMessage,
                confirmText: 'Delete',
                confirmStyle: 'danger',
                onConfirm: async () => {
                    try {
                        await deleteFn(resource);
                        Logger.info(`${resourceLabel} deleted successfully`, { namespace, name });
                        closeModal();
                    } catch (err) {
                        Logger.error(`Failed to delete ${resourceLabel}`, err);
                        addNotification({ type: 'error', title: `Failed to delete ${resourceLabel.toLowerCase()}`, message: String(err.message || err) });
                    }
                }
            });
        };
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        // Expose context for custom handlers
        openTab,
        closeTab,
        openModal,
        closeModal,
        currentContext,
        addNotification,
    };
}
