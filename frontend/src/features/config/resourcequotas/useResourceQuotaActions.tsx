import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import { DeleteResourceQuota } from '../../../../wailsjs/go/main/App';
import ResourceQuotaDetails from '../../../components/shared/ResourceQuotaDetails';
import { K8sResourceQuota } from '../../../types/k8s';

export interface ResourceQuotaActionsReturn extends BaseResourceActionsReturn<K8sResourceQuota> {
    handleDelete: (quota: K8sResourceQuota) => void;
}

export const useResourceQuotaActions = (): ResourceQuotaActionsReturn => {
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
        async (quota: K8sResourceQuota): Promise<void> => {
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
