import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteLimitRange } from '../../../../wailsjs/go/main/App';
import LimitRangeDetails from '../../../components/shared/LimitRangeDetails';

export const useLimitRangeActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'limitrange',
        resourceLabel: 'LimitRange',
        DetailsComponent: LimitRangeDetails,
        detailsPropName: 'limitRange',
    });

    const handleDelete = createDeleteHandler(
        async (lr) => {
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
