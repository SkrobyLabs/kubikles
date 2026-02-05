import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import { DeletePV } from '../../../../wailsjs/go/main/App';
import PVDetails from '../../../components/shared/PVDetails';
import { K8sPersistentVolume } from '../../../types/k8s';

export interface PVActionsReturn extends BaseResourceActionsReturn<K8sPersistentVolume> {
    handleDelete: (pv: K8sPersistentVolume) => void;
}

export const usePVActions = (): PVActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'pv',
        resourceLabel: 'PersistentVolume',
        DetailsComponent: PVDetails,
        detailsPropName: 'pv',
        isNamespaced: false,
    });

    const handleDelete = createDeleteHandler(async (pv: K8sPersistentVolume): Promise<void> => {
        await DeletePV(pv.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
