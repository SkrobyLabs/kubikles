import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteIngressClass } from '../../../../wailsjs/go/main/App';
import { LazyYamlEditor as YamlEditor } from '../../../components/lazy';
import Logger from '../../../utils/Logger';
import { TagIcon, PencilSquareIcon } from '@heroicons/react/24/outline';

export const useIngressClassActions = () => {
    const { openTab, closeTab, showConfirm } = useUI();
    const { currentContext, triggerRefresh } = useK8s();

    const handleEditYaml = (ingressClass) => {
        Logger.info("Opening YAML editor for IngressClass", { name: ingressClass.metadata.name });
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
            )
        });
    };

    const handleDelete = (ingressClass) => {
        Logger.info("Delete requested for IngressClass", { name: ingressClass.metadata.name });
        showConfirm({
            title: 'Delete Ingress Class',
            message: `Are you sure you want to delete ingress class "${ingressClass.metadata.name}"?`,
            confirmLabel: 'Delete',
            cancelLabel: 'Cancel',
            onConfirm: async () => {
                try {
                    await DeleteIngressClass(ingressClass.metadata.name);
                    Logger.info("IngressClass deleted successfully", { name: ingressClass.metadata.name });
                    triggerRefresh();
                } catch (err) {
                    Logger.error("Failed to delete ingress class", { error: err });
                }
            }
        });
    };

    return {
        handleEditYaml,
        handleDelete
    };
};
