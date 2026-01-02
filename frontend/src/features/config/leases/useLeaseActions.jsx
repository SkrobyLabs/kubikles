import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteLease } from '../../../../wailsjs/go/main/App';
import LeaseDetails from '../../../components/shared/LeaseDetails';

export const useLeaseActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'lease',
        resourceLabel: 'Lease',
        DetailsComponent: LeaseDetails,
        detailsPropName: 'lease',
    });

    const handleDelete = createDeleteHandler(
        async (lease) => {
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
