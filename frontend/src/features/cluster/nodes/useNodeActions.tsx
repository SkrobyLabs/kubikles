import React, { useCallback } from 'react';
import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { useK8s } from '~/context';
import { DeleteNode, SetNodeSchedulable } from 'wailsjs/go/main/App';
import NodeDetails from '~/components/shared/NodeDetails';
import NodeShellTab from './NodeShellTab';
import Logger from '~/utils/Logger';
import { ServerIcon, CommandLineIcon } from '@heroicons/react/24/outline';
import { K8sNode } from '~/types/k8s';

export interface NodeActionsReturn extends BaseResourceActionsReturn<K8sNode> {
    handleCordonUncordon: (node: K8sNode) => Promise<void>;
    handleShell: (node: K8sNode) => void;
    handleDelete: (node: K8sNode) => void;
}

export const useNodeActions = (refetch?: () => void): any => {
    const { currentContext } = useK8s();
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
        openTab,

        addNotification,
    } = useBaseResourceActions<K8sNode>({
        resourceType: 'node',
        resourceLabel: 'Node',
        DetailsComponent: NodeDetails,
        detailsPropName: 'node',
        isNamespaced: false,
        hasDependencies: false,
    });

    const handleCordonUncordon = useCallback(async (node: K8sNode): Promise<void> => {
        const isUnschedulable = node.spec?.unschedulable === true;
        const action = isUnschedulable ? 'Uncordon' : 'Cordon';
        const name = node.metadata.name;

        Logger.info(`${action}ing node`, { name }, 'k8s');

        try {
            await SetNodeSchedulable(name, isUnschedulable);
            Logger.info(`Node ${action.toLowerCase()}ed successfully`, { name }, 'k8s');
            if (refetch) refetch();
        } catch (err: any) {
            Logger.error(`Failed to ${action.toLowerCase()} node`, err, 'k8s');
            addNotification({ type: 'error', title: `Failed to ${action.toLowerCase()} node`, message: String(err) });
        }
    }, [refetch, addNotification]);

    const handleShell = useCallback((node: K8sNode): void => {
        const nodeName = node.metadata.name;
        const tabId = `terminal-node-${node.metadata.name}`;
        Logger.info("Opening shell on node", { node: nodeName }, 'k8s');

        openTab({
            id: tabId,
            title: nodeName,
            icon: ServerIcon,
            actionLabel: 'Shell',
            keepAlive: true,
            content: <NodeShellTab nodeName={nodeName} context={currentContext} />,
            resourceMeta: { kind: 'Node', name: nodeName },
        });
    }, [currentContext, openTab]);

    const handleDelete = createDeleteHandler(
        async (node: any): Promise<void> => {
            await DeleteNode(node.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this node? This will remove the node from the cluster. Any pods running on this node will need to be rescheduled.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleCordonUncordon,
        handleShell,
        handleDelete
    };
};
