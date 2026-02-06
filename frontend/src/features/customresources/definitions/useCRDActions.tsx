import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { DeleteCRD } from 'wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '~/components/lazy';
import Logger from '~/utils/Logger';
import { K8sCustomResourceDefinition } from '~/types/k8s';
import { BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';

/**
 * Return type for useCRDActions
 */
export interface CRDActionsReturn extends Pick<BaseResourceActionsReturn<K8sCustomResourceDefinition>, 'handleEditYaml'> {
    handleDelete: (crd: K8sCustomResourceDefinition) => void;
}

export const useCRDActions = (): any => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (crd: K8sCustomResourceDefinition): void => {
        Logger.info("Opening CRD YAML editor", { name: crd.metadata.name });
        const tabId = `crd-${crd.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${crd.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="crd"
                    resourceName={crd.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'CustomResourceDefinition', name: crd.metadata.name },
        });
    };

    const handleDelete = (crd: K8sCustomResourceDefinition): void => {
        const name = crd.metadata.name;
        Logger.info("Delete CRD requested", { name });

        openModal({
            title: `Delete CRD ${name}?`,
            content: `Are you sure you want to delete CustomResourceDefinition "${name}"? This will remove the CRD and all custom resources of this type from the cluster. This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await DeleteCRD(name);
                    Logger.info("CRD deleted successfully", { name });
                    closeModal();
                } catch (err: any) {
                    Logger.error("Failed to delete CRD", err);
                    addNotification({ type: 'error', title: 'Failed to delete CRD', message: String(err) });
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
