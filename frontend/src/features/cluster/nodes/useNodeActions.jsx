import React, { useCallback } from 'react';
import { useUI } from '../../../context/UIContext';
import { useK8s } from '../../../context/K8sContext';
import {
    DeleteNode,
    SetNodeSchedulable
} from '../../../../wailsjs/go/main/App';
import YamlEditor from '../../../components/shared/YamlEditor';
import NodeDetails from '../../../components/shared/NodeDetails';
import NodeShellTab from './NodeShellTab';
import Logger from '../../../utils/Logger';

export const useNodeActions = (refetch) => {
    const { openTab, closeTab, openModal, closeModal } = useUI();
    const { currentContext } = useK8s();

    const handleShowDetails = useCallback((node) => {
        Logger.info("Opening node details", { name: node.metadata.name });
        const tabId = `details-node-${node.metadata.uid}`;
        openTab({
            id: tabId,
            title: `${node.metadata.name}`,
            content: (
                <NodeDetails
                    node={node}
                    tabContext={currentContext}
                />
            )
        });
    }, [openTab, currentContext]);

    const handleEditYaml = useCallback((node) => {
        Logger.info("Opening YAML editor for Node", { name: node.metadata.name });
        const tabId = `yaml-node-${node.metadata.uid}`;
        openTab({
            id: tabId,
            title: `Edit: ${node.metadata.name}`,
            content: (
                <YamlEditor
                    resourceType="node"
                    namespace=""
                    resourceName={node.metadata.name}
                    onClose={() => closeTab(tabId)}
                    tabContext={currentContext}
                />
            )
        });
    }, [openTab, closeTab]);

    const handleCordonUncordon = useCallback(async (node) => {
        const isUnschedulable = node.spec?.unschedulable === true;
        const action = isUnschedulable ? 'Uncordon' : 'Cordon';
        const name = node.metadata.name;

        Logger.info(`${action}ing node`, { name });

        try {
            // schedulable = true means uncordon, false means cordon
            await SetNodeSchedulable(name, isUnschedulable);
            Logger.info(`Node ${action.toLowerCase()}ed successfully`, { name });
            // Refresh the node list to reflect the change
            if (refetch) refetch();
        } catch (err) {
            Logger.error(`Failed to ${action.toLowerCase()} node`, err);
            alert(`Failed to ${action.toLowerCase()} node: ${err}`);
        }
    }, [refetch]);

    const handleShell = useCallback((node) => {
        const nodeName = node.metadata.name;
        const tabId = `node-shell-${node.metadata.uid}`;
        Logger.info("Opening shell on node", { node: nodeName });

        openTab({
            id: tabId,
            title: `Shell: ${nodeName}`,
            content: <NodeShellTab nodeName={nodeName} context={currentContext} />
        });
    }, [currentContext, openTab]);

    const handleDelete = useCallback((node) => {
        const name = node.metadata.name;
        Logger.info("Delete Node requested", { name });

        openModal({
            title: `Delete Node ${name}?`,
            content: `Are you sure you want to delete node "${name}"? This will remove the node from the cluster. Any pods running on this node will need to be rescheduled.`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteNode(name);
                    Logger.info("Node deleted successfully", { name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete node", err);
                    alert(`Failed to delete node: ${err}`);
                }
            }
        });
    }, [openModal, closeModal]);

    return {
        handleShowDetails,
        handleEditYaml,
        handleCordonUncordon,
        handleShell,
        handleDelete
    };
};
