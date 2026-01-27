import React, { useCallback } from 'react';
import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteNode, SetNodeSchedulable } from '../../../../wailsjs/go/main/App';
import NodeDetails from '../../../components/shared/NodeDetails';
import NodeShellTab from './NodeShellTab';
import Logger from '../../../utils/Logger';
import { ServerIcon, CommandLineIcon } from '@heroicons/react/24/outline';

export const useNodeActions = (refetch) => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
        openTab,
        currentContext,
        addNotification,
    } = useBaseResourceActions({
        resourceType: 'node',
        resourceLabel: 'Node',
        DetailsComponent: NodeDetails,
        detailsPropName: 'node',
        isNamespaced: false,
        hasDependencies: false,
    });

    const handleCordonUncordon = useCallback(async (node) => {
        const isUnschedulable = node.spec?.unschedulable === true;
        const action = isUnschedulable ? 'Uncordon' : 'Cordon';
        const name = node.metadata.name;

        Logger.info(`${action}ing node`, { name });

        try {
            await SetNodeSchedulable(name, isUnschedulable);
            Logger.info(`Node ${action.toLowerCase()}ed successfully`, { name });
            if (refetch) refetch();
        } catch (err) {
            Logger.error(`Failed to ${action.toLowerCase()} node`, err);
            addNotification({ type: 'error', title: `Failed to ${action.toLowerCase()} node`, message: String(err) });
        }
    }, [refetch]);

    const handleShell = useCallback((node) => {
        const nodeName = node.metadata.name;
        const tabId = `terminal-node-${node.metadata.uid}`;
        Logger.info("Opening shell on node", { node: nodeName });

        openTab({
            id: tabId,
            title: nodeName,
            icon: ServerIcon,
            actionLabel: 'Shell',
            keepAlive: true,
            content: <NodeShellTab nodeName={nodeName} context={currentContext} />
        });
    }, [currentContext, openTab]);

    const handleDelete = createDeleteHandler(
        async (node) => {
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
