import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteStorageClass } from '../../../../wailsjs/go/main/App';
import StorageClassDetails from '../../../components/shared/StorageClassDetails';

export const useStorageClassActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'storageclass',
        resourceLabel: 'StorageClass',
        DetailsComponent: StorageClassDetails,
        detailsPropName: 'storageClass',
        isNamespaced: false,
        hasDependencies: false,
    });

    const handleDelete = createDeleteHandler(async (storageClass) => {
        await DeleteStorageClass(storageClass.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
