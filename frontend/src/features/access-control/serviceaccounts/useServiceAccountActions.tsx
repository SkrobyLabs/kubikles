import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteServiceAccount } from 'wailsjs/go/main/App';
import ServiceAccountDetails from '~/components/shared/ServiceAccountDetails';
import { K8sServiceAccount } from '~/types/k8s';

export interface ServiceAccountActionsReturn extends BaseResourceActionsReturn<K8sServiceAccount> {
    handleDelete: (serviceAccount: K8sServiceAccount) => void;
}

export const useServiceAccountActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'serviceaccount',
        resourceLabel: 'ServiceAccount',
        DetailsComponent: ServiceAccountDetails,
        detailsPropName: 'serviceAccount',
    });

    const handleDelete = createDeleteHandler(
        async (serviceAccount: any): Promise<void> => {
            await DeleteServiceAccount(serviceAccount.metadata.namespace, serviceAccount.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this ServiceAccount? Pods using this account may lose access to cluster resources.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
