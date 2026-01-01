import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeletePDB } from '../../../../wailsjs/go/main/App';
import PDBDetails from '../../../components/shared/PDBDetails';

export const usePDBActions = () => {
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
        async (pdb) => {
            await DeletePDB(currentContext, pdb.metadata.namespace, pdb.metadata.name);
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
