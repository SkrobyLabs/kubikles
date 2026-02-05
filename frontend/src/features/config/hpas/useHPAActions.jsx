import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteHPA } from '../../../../wailsjs/go/main/App';
import HPADetails from '../../../components/shared/HPADetails';

export const useHPAActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'hpa',
        resourceLabel: 'HorizontalPodAutoscaler',
        DetailsComponent: HPADetails,
        detailsPropName: 'hpa',
    });

    const handleDelete = createDeleteHandler(
        async (hpa) => {
            await DeleteHPA(hpa.metadata.namespace, hpa.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this HorizontalPodAutoscaler? Automatic scaling will be disabled.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
