import React from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import { DeleteIngress } from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import DependencyGraph from '../../../components/shared/DependencyGraph';
import IngressDetails from '../../../components/shared/IngressDetails';
import Logger from '../../../utils/Logger';

export const useIngressActions = () => {
    const { openTab, closeTab, showConfirm } = useUI();
    const { currentContext, triggerRefresh } = useK8s();

    const handleShowDetails = (ingress) => {
        Logger.info("Opening Ingress details", { namespace: ingress.metadata.namespace, name: ingress.metadata.name });
        const tabId = `details-ingress-${ingress.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${ingress.metadata.name}`,
            content: (
                <IngressDetails
                    ingress={ingress}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleEditYaml = (ingress) => {
        Logger.info("Opening YAML editor for Ingress", { namespace: ingress.metadata.namespace, name: ingress.metadata.name });
        const tabId = `yaml-ingress-${ingress.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${ingress.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="ingress"
                    namespace={ingress.metadata.namespace}
                    resourceName={ingress.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    };

    const handleShowDependencies = (ingress) => {
        Logger.info("Opening dependency graph", { namespace: ingress.metadata.namespace, ingress: ingress.metadata.name });
        const tabId = `deps-ingress-${ingress.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Deps: ${ingress.metadata.name}`,
            content: (
                <DependencyGraph
                    resourceType="ingress"
                    namespace={ingress.metadata.namespace}
                    resourceName={ingress.metadata.name}
                    onClose={() => closeTab(tabId)}
                />
            )
        });
    };

    const handleDelete = (ingress) => {
        Logger.info("Delete requested for Ingress", { namespace: ingress.metadata.namespace, name: ingress.metadata.name });
        showConfirm({
            title: 'Delete Ingress',
            message: `Are you sure you want to delete ingress "${ingress.metadata.name}" in namespace "${ingress.metadata.namespace}"?`,
            confirmLabel: 'Delete',
            cancelLabel: 'Cancel',
            onConfirm: async () => {
                try {
                    await DeleteIngress(ingress.metadata.namespace, ingress.metadata.name);
                    Logger.info("Ingress deleted successfully", { namespace: ingress.metadata.namespace, name: ingress.metadata.name });
                    triggerRefresh();
                } catch (err) {
                    Logger.error("Failed to delete ingress", { error: err });
                }
            }
        });
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
