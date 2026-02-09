import React from 'react';
import { useUI } from '~/context';
import { useK8s } from '~/context';
import { DeleteIngressClass } from 'wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '~/components/lazy';
import Logger from '~/utils/Logger';
import { TagIcon, PencilSquareIcon } from '@heroicons/react/24/outline';
import { K8sIngressClass } from '~/types/k8s';

export interface IngressClassActionsReturn {
    handleEditYaml: (ingressClass: K8sIngressClass) => void;
    handleDelete: (ingressClass: K8sIngressClass) => void;
}

export const useIngressClassActions = (): any => {
    const { openTab, closeTab, openModal } = useUI();
    const { currentContext, triggerRefresh } = useK8s();

    const handleEditYaml = (ingressClass: K8sIngressClass): void => {
        Logger.info("Opening YAML editor for IngressClass", { name: ingressClass.metadata.name }, 'k8s');
        const tabId = `yaml-ingressclass-${ingressClass.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${ingressClass.metadata.name}`,
            icon: TagIcon,
            actionLabel: 'Edit',
            content: (
                <YamlEditor
                    resourceType="ingressclass"
                    namespace=""
                    resourceName={ingressClass.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'IngressClass', name: ingressClass.metadata.name },
        });
    };

    const handleDelete = (ingressClass: K8sIngressClass): void => {
        Logger.info("Delete requested for IngressClass", { name: ingressClass.metadata.name }, 'k8s');
        openModal({
            title: 'Delete Ingress Class',
            content: `Are you sure you want to delete ingress class "${ingressClass.metadata.name}"?`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async (): Promise<void> => {
                try {
                    await DeleteIngressClass(ingressClass.metadata.name);
                    Logger.info("IngressClass deleted successfully", { name: ingressClass.metadata.name }, 'k8s');
                    triggerRefresh();
                } catch (err: any) {
                    Logger.error("Failed to delete ingress class", { error: err }, 'k8s');
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
