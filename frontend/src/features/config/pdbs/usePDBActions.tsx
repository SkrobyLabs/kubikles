import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import { DeletePDB } from '../../../../wailsjs/go/main/App';
import PDBDetails from '../../../components/shared/PDBDetails';
import { K8sPodDisruptionBudget } from '../../../types/k8s';

export interface PDBActionsReturn extends BaseResourceActionsReturn<K8sPodDisruptionBudget> {
    handleDelete: (pdb: K8sPodDisruptionBudget) => void;
}

export const usePDBActions = (): PDBActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'pdb',
        resourceLabel: 'PodDisruptionBudget',
        DetailsComponent: PDBDetails,
        detailsPropName: 'pdb',
    });

    const handleDelete = createDeleteHandler(
        async (pdb: K8sPodDisruptionBudget): Promise<void> => {
            await DeletePDB(pdb.metadata.namespace, pdb.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this PodDisruptionBudget? Pod disruption protection will be removed.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
