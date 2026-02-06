import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteCSINode } from 'wailsjs/go/main/App';
import CSINodeDetails from '~/components/shared/CSINodeDetails';
import { K8sCSINode } from '~/types/k8s';

export interface CSINodeActionsReturn extends BaseResourceActionsReturn<K8sCSINode> {
    handleDelete: (csiNode: K8sCSINode) => void;
}

export const useCSINodeActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
    } = useBaseResourceActions<K8sCSINode>({
        resourceType: 'csinode',
        resourceLabel: 'CSINode',
        DetailsComponent: CSINodeDetails,
        detailsPropName: 'csiNode',
    });

    const handleDelete = createDeleteHandler(
        async (csiNode: any): Promise<void> => {
            await DeleteCSINode(csiNode.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this CSI Node? This is usually managed automatically by the kubelet.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
