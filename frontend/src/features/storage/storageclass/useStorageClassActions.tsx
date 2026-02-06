import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteStorageClass } from 'wailsjs/go/main/App';
import StorageClassDetails from '~/components/shared/StorageClassDetails';
import { K8sStorageClass } from '~/types/k8s';

export interface StorageClassActionsReturn extends BaseResourceActionsReturn<K8sStorageClass> {
    handleDelete: (storageClass: K8sStorageClass) => void;
}

export const useStorageClassActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,

    } = useBaseResourceActions({
        resourceType: 'storageclass',
        resourceLabel: 'StorageClass',
        DetailsComponent: StorageClassDetails,
        detailsPropName: 'storageClass',
        isNamespaced: false,
        hasDependencies: false,
    });

    const handleDelete = createDeleteHandler(
        async (storageClass: any): Promise<void> => {
        await DeleteStorageClass(storageClass.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
