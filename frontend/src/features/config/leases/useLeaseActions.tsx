import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteLease } from 'wailsjs/go/main/App';
import LeaseDetails from '~/components/shared/LeaseDetails';
import { K8sLease } from '~/types/k8s';

export interface LeaseActionsReturn extends BaseResourceActionsReturn<K8sLease> {
    handleDelete: (lease: K8sLease) => void;
}

export const useLeaseActions = (): LeaseActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions({
        resourceType: 'lease',
        resourceLabel: 'Lease',
        DetailsComponent: LeaseDetails,
        detailsPropName: 'lease',
    });

    const handleDelete = createDeleteHandler(
        async (lease: K8sLease): Promise<void> => {
            await DeleteLease(lease.metadata.namespace, lease.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this Lease? This may affect leader election in the cluster.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
