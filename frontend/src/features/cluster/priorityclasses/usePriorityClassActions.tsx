import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import { DeletePriorityClass } from '../../../../wailsjs/go/main/App';
import PriorityClassDetails from '../../../components/shared/PriorityClassDetails';
import { K8sPriorityClass } from '../../../types/k8s';

export interface PriorityClassActionsReturn extends BaseResourceActionsReturn<K8sPriorityClass> {
    handleDelete: (priorityClass: K8sPriorityClass) => void;
}

export const usePriorityClassActions = (): PriorityClassActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions<K8sPriorityClass>({
        resourceType: 'priorityclass',
        resourceLabel: 'Priority Class',
        DetailsComponent: PriorityClassDetails,
        detailsPropName: 'priorityClass',
    });

    const handleDelete = createDeleteHandler(
        async (priorityClass: K8sPriorityClass): Promise<void> => {
            await DeletePriorityClass(priorityClass.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this priority class? This may affect pod scheduling priorities.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
