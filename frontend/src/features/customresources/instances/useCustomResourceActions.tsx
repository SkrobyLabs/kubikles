import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import {
    GetCustomResourceYaml,
    UpdateCustomResourceYaml,
    DeleteCustomResource
} from 'wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '~/components/lazy';
import CustomResourceDetails from '~/components/shared/CustomResourceDetails';
import { K8sResource } from '~/types/k8s';
import { getResourceIcon } from '~/utils/resourceIcons';

/**
 * CRD Information for custom resource operations
 */
export interface CRDInfo {
    group: string;
    version: string;
    resource: string;
    kind: string;
    namespaced: boolean;
}

/**
 * Return type for useCustomResourceActions
 */
export interface CustomResourceActionsReturn {
    handleShowDetails: (resource: K8sResource) => void;
    handleEditYaml: (resource: K8sResource) => void;
    handleDelete: (resource: K8sResource) => void;
}

/**
 * Hook for custom resource instance actions (edit, delete, show details)
 */
export const useCustomResourceActions = (crdInfo: CRDInfo): any => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleShowDetails = (resource: K8sResource): void => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace || '';
        const tabId = `details-cr-${crdInfo.group}-${crdInfo.resource}-${namespace ? `${namespace}/${name}` : name}`;

        openTab({
            id: tabId,
            title: name || 'Unknown',
            icon: getResourceIcon('customresource'),
            content: (
                <CustomResourceDetails
                    resource={resource}
                    crdInfo={crdInfo}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: crdInfo.kind, name, namespace: namespace || undefined },
        });
    };

    const handleEditYaml = (resource: K8sResource): void => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace || '';
        const tabId = `cr-yaml-${crdInfo.group}-${crdInfo.resource}-${namespace}-${name}`;

        openTab({
            id: tabId,
            title: `${name} (${crdInfo.kind})`,
            content: (
                <YamlEditor
                    resourceType="customresource"
                    namespace={namespace}
                    resourceName={name}
                    onClose={() => closeTab(tabId)}
                    getYamlFn={() => GetCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, namespace, name)}
                    updateYamlFn={(content: string) => UpdateCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, namespace, name, content)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: crdInfo.kind, name, namespace: namespace || undefined },
        });
    };

    const handleDelete = (resource: K8sResource): void => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace || '';
        const displayName = namespace ? `${namespace}/${name}` : name;

        openModal({
            title: `Delete ${crdInfo.kind}`,
            content: `Are you sure you want to delete ${crdInfo.kind} "${displayName}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await DeleteCustomResource(crdInfo.group, crdInfo.version, crdInfo.resource, namespace, name);
                    closeModal();
                } catch (err: any) {
                    console.error(`Failed to delete ${crdInfo.kind}:`, err);
                    // The modal will stay open so user can see the error or retry
                }
            },
            onClose: (): void => closeModal()
        });
    };

    return { handleShowDetails, handleEditYaml, handleDelete };
};
