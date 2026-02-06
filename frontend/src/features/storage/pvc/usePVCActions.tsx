import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeletePVC } from 'wailsjs/go/main/App';
import PVCDetails from '~/components/shared/PVCDetails';
import { K8sPersistentVolumeClaim } from '~/types/k8s';

export interface PVCActionsReturn extends BaseResourceActionsReturn<K8sPersistentVolumeClaim> {
    handleDelete: (pvc: K8sPersistentVolumeClaim) => void;
}

export const usePVCActions = (): any => {
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

    const handleDelete = createDeleteHandler(
        async (pvc: any): Promise<void> => {
        await DeletePVC(pvc.metadata.namespace, pvc.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
