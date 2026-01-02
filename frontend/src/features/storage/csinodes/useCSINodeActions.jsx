import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteCSINode } from '../../../../wailsjs/go/main/App';
import CSINodeDetails from '../../../components/shared/CSINodeDetails';

export const useCSINodeActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'csinode',
        resourceLabel: 'CSINode',
        DetailsComponent: CSINodeDetails,
        detailsPropName: 'csiNode',
    });

    const handleDelete = createDeleteHandler(
        async (csiNode) => {
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
