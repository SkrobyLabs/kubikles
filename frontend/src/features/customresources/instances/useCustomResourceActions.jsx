import React from 'react';
import { useUI } from '../../../context';
import { useK8s } from '../../../context';
import {
    GetCustomResourceYaml,
    UpdateCustomResourceYaml,
    DeleteCustomResource
} from '../../../../wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '../../../components/lazy';

/**
 * Hook for custom resource instance actions (edit, delete)
 * @param {Object} crdInfo - Info about the CRD: { group, version, resource, kind, namespaced }
 */
export const useCustomResourceActions = (crdInfo) => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleEditYaml = (resource) => {
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
                    updateYamlFn={(content) => UpdateCustomResourceYaml(crdInfo.group, crdInfo.version, crdInfo.resource, namespace, name, content)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: crdInfo.kind, name, namespace: namespace || undefined },
        });
    };

    const handleDelete = (resource) => {
        const name = resource.metadata?.name;
        const namespace = resource.metadata?.namespace || '';
        const displayName = namespace ? `${namespace}/${name}` : name;

        openModal({
            title: `Delete ${crdInfo.kind}`,
            message: `Are you sure you want to delete ${crdInfo.kind} "${displayName}"? This action cannot be undone.`,
            confirmLabel: 'Delete',
            confirmVariant: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteCustomResource(crdInfo.group, crdInfo.version, crdInfo.resource, namespace, name);
                    closeModal();
                } catch (err) {
                    console.error(`Failed to delete ${crdInfo.kind}:`, err);
                    // The modal will stay open so user can see the error or retry
                }
            },
            onCancel: () => closeModal()
        });
    };

    return { handleEditYaml, handleDelete };
};
