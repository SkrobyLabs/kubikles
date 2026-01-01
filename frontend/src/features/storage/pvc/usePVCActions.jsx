import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeletePVC } from '../../../../wailsjs/go/main/App';
import PVCDetails from '../../../components/shared/PVCDetails';

export const usePVCActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'pvc',
        resourceLabel: 'PersistentVolumeClaim',
        DetailsComponent: PVCDetails,
        detailsPropName: 'pvc',
    });

    const handleDelete = createDeleteHandler(async (pvc) => {
        await DeletePVC(pvc.metadata.namespace, pvc.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
