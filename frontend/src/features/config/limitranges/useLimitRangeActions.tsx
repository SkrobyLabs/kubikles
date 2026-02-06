import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteLimitRange } from 'wailsjs/go/main/App';
import LimitRangeDetails from '~/components/shared/LimitRangeDetails';
import { K8sLimitRange } from '~/types/k8s';

export interface LimitRangeActionsReturn extends BaseResourceActionsReturn<K8sLimitRange> {
    handleDelete: (limitRange: K8sLimitRange) => void;
}

export const useLimitRangeActions = (): LimitRangeActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions({
        resourceType: 'limitrange',
        resourceLabel: 'LimitRange',
        DetailsComponent: LimitRangeDetails,
        detailsPropName: 'limitRange',
    });

    const handleDelete = createDeleteHandler(
        async (lr: K8sLimitRange): Promise<void> => {
            await DeleteLimitRange(lr.metadata.namespace, lr.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this LimitRange? Default limits will no longer be applied.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
