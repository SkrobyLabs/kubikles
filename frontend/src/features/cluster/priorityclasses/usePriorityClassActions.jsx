import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeletePriorityClass } from '../../../../wailsjs/go/main/App';
import PriorityClassDetails from '../../../components/shared/PriorityClassDetails';

export const usePriorityClassActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'priorityclass',
        resourceLabel: 'Priority Class',
        DetailsComponent: PriorityClassDetails,
        detailsPropName: 'priorityClass',
    });

    const handleDelete = createDeleteHandler(
        async (priorityClass) => {
            await DeletePriorityClass(currentContext, priorityClass.metadata.name);
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
