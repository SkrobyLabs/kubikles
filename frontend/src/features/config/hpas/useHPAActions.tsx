import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteHPA } from 'wailsjs/go/main/App';
import HPADetails from '~/components/shared/HPADetails';
import { K8sHorizontalPodAutoscaler } from '~/types/k8s';

export interface HPAActionsReturn extends BaseResourceActionsReturn<K8sHorizontalPodAutoscaler> {
    handleDelete: (hpa: K8sHorizontalPodAutoscaler) => void;
}

export const useHPAActions = (): HPAActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions({
        resourceType: 'hpa',
        resourceLabel: 'HorizontalPodAutoscaler',
        DetailsComponent: HPADetails,
        detailsPropName: 'hpa',
    });

    const handleDelete = createDeleteHandler(
        async (hpa: K8sHorizontalPodAutoscaler): Promise<void> => {
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
