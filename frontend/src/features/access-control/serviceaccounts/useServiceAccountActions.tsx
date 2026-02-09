import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import { DeleteServiceAccount } from 'wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '~/components/lazy';
import Logger from '~/utils/Logger';
import { K8sServiceAccount } from '~/types/k8s';
import { BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';

/**
 * Return type for useServiceAccountActions
 */
export interface ServiceAccountActionsReturn extends Pick<BaseResourceActionsReturn<K8sServiceAccount>, 'handleEditYaml'> {
    handleDelete: (serviceAccount: K8sServiceAccount) => void;
}

export const useServiceAccountActions = (): any => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();

    const handleEditYaml = (serviceAccount: K8sServiceAccount): void => {
        Logger.info("Opening ServiceAccount editor", { namespace: serviceAccount.metadata.namespace, name: serviceAccount.metadata.name }, 'k8s');
        const tabId = `serviceaccount-${serviceAccount.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${serviceAccount.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="serviceaccount"
                    namespace={serviceAccount.metadata.namespace}
                    resourceName={serviceAccount.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'ServiceAccount', name: serviceAccount.metadata.name, namespace: serviceAccount.metadata.namespace },
        });
    };

    const handleDelete = (serviceAccount: K8sServiceAccount): void => {
        const name = serviceAccount.metadata.name;
        const namespace = serviceAccount.metadata.namespace;
        Logger.info("Delete ServiceAccount requested", { namespace, name }, 'k8s');

        openModal({
            title: `Delete ServiceAccount ${name}?`,
            content: `Are you sure you want to delete service account "${name}" in namespace "${namespace}"? This action cannot be undone.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await DeleteServiceAccount(namespace, name);
                    Logger.info("ServiceAccount deleted successfully", { namespace, name }, 'k8s');
                    closeModal();
                } catch (err: any) {
                    Logger.error("Failed to delete ServiceAccount", err, 'k8s');
                    addNotification({ type: 'error', title: 'Failed to delete service account', message: String(err) });
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
