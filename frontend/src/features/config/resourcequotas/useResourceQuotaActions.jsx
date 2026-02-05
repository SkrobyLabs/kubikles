import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteResourceQuota } from '../../../../wailsjs/go/main/App';
import ResourceQuotaDetails from '../../../components/shared/ResourceQuotaDetails';

export const useResourceQuotaActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'resourcequota',
        resourceLabel: 'ResourceQuota',
        DetailsComponent: ResourceQuotaDetails,
        detailsPropName: 'resourceQuota',
    });

    const handleDelete = createDeleteHandler(
        async (quota) => {
            await DeleteResourceQuota(quota.metadata.namespace, quota.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this ResourceQuota? Resource limits will no longer be enforced.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
