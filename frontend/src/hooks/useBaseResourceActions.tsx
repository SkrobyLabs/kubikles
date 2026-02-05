import React from 'react';
import { useUI } from '../context';
import { useK8s } from '../context';
import { useNotification } from '../context';
import { LazyYamlEditor, LazyDependencyGraph } from '../components/lazy';
import Logger from '../utils/Logger';
import { getResourceIcon } from '../utils/resourceIcons';
import { K8sResource } from '../types/k8s';

/**
 * Configuration for useBaseResourceActions hook
 */
export interface BaseResourceActionsConfig<T extends K8sResource = K8sResource> {
  /** Resource type identifier (e.g., 'deployment', 'configmap') */
  resourceType: string;
  /** Human-readable label (e.g., 'Deployment', 'ConfigMap') */
  resourceLabel: string;
  /** Component to render for details view */
  DetailsComponent: React.ComponentType<any>;
  /** Prop name for passing resource to DetailsComponent (e.g., 'deployment') */
  detailsPropName: string;
  /** Whether the resource is namespaced (default: true) */
  isNamespaced?: boolean;
  /** Whether to include dependency graph action (default: true) */
  hasDependencies?: boolean;
  /** Custom function to get tab title from resource (defaults to metadata.name) */
  getTabTitle?: (resource: T) => string | undefined;
}

/**
 * Options for delete handler
 */
export interface DeleteHandlerOptions {
  /** Custom confirmation message */
  confirmMessage?: string;
}

/**
 * Return type for useBaseResourceActions
 */
export interface BaseResourceActionsReturn<T extends K8sResource = K8sResource> {
  handleShowDetails: (resource: T) => void;
  handleEditYaml: (resource: T) => void;
  handleShowDependencies: ((resource: T) => void) | undefined;
  createDeleteHandler: (
    deleteFn: (resource: T) => Promise<void>,
    options?: DeleteHandlerOptions
  ) => (resource: T) => void;
  // Exposed context for custom handlers
  openTab: ReturnType<typeof useUI>['openTab'];
  closeTab: ReturnType<typeof useUI>['closeTab'];
  openModal: ReturnType<typeof useUI>['openModal'];
  closeModal: ReturnType<typeof useUI>['closeModal'];
  currentContext: string;
  addNotification: ReturnType<typeof useNotification>['addNotification'];
}

/**
 * Creates standard resource action handlers for showing details, editing YAML,
 * showing dependencies, and deleting resources.
 *
 * @param config - Configuration object
 * @returns Object containing action handlers
 */
export function useBaseResourceActions<T extends K8sResource = K8sResource>(
  config: BaseResourceActionsConfig<T>
): BaseResourceActionsReturn<T> {
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
    getTabTitle = (resource: T) => resource.metadata?.name,
  } = config;

  /**
   * Opens a details tab for the resource
   */
  const handleShowDetails = (resource: T): void => {
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
      title: getTabTitle(resource) || name || 'Unknown',
      icon: getResourceIcon(resourceType),
      content: <DetailsComponent {...props} />,
      resourceMeta: { kind: resourceLabel, name, namespace },
    });
  };

  /**
   * Opens a YAML editor tab for the resource
   */
  const handleEditYaml = (resource: T): void => {
    const name = resource.metadata?.name;
    const namespace = resource.metadata?.namespace;
    Logger.info(`Opening ${resourceLabel} YAML editor`, { namespace, name });

    const tabId = `yaml-${resourceType}-${resource.metadata.uid}`;
    openTab({
      id: tabId,
      title: name || 'Unknown',
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
  const handleShowDependencies = hasDependencies
    ? (resource: T): void => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace;
        Logger.info(`Opening ${resourceLabel} dependency graph`, {
          namespace,
          name,
        });

        const tabId = `deps-${resourceType}-${resource.metadata.uid}`;
        openTab({
          id: tabId,
          title: name || 'Unknown',
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
      }
    : undefined;

  /**
   * Creates a delete handler with confirmation modal
   * @param deleteFn - The delete function to call
   * @param options - Additional options
   * @returns Delete handler
   */
  const createDeleteHandler = (
    deleteFn: (resource: T) => Promise<void>,
    options: DeleteHandlerOptions = {}
  ): ((resource: T) => void) => {
    return (resource: T): void => {
      const name = resource.metadata?.name;
      const namespace = resource.metadata?.namespace;
      Logger.info(`Delete ${resourceLabel} requested`, { namespace, name });

      const confirmMessage =
        options.confirmMessage ||
        `Are you sure you want to delete ${resourceLabel.toLowerCase()} "${name}"? This action cannot be undone.`;

      openModal({
        title: `Delete ${resourceLabel} ${name}?`,
        content: confirmMessage,
        confirmText: 'Delete',
        confirmStyle: 'danger',
        onConfirm: async () => {
          try {
            await deleteFn(resource);
            Logger.info(`${resourceLabel} deleted successfully`, {
              namespace,
              name,
            });
            closeModal();
          } catch (err: any) {
            Logger.error(`Failed to delete ${resourceLabel}`, err);
            addNotification({
              type: 'error',
              title: `Failed to delete ${resourceLabel.toLowerCase()}`,
              message: String(err.message || err),
            });
          }
        },
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
