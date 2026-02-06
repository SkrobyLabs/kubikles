import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteNetworkPolicy } from 'wailsjs/go/main/App';
import NetworkPolicyDetails from '~/components/shared/NetworkPolicyDetails';
import { K8sNetworkPolicy } from '~/types/k8s';

export interface NetworkPolicyActionsReturn extends BaseResourceActionsReturn<K8sNetworkPolicy> {
    handleDelete: (networkPolicy: K8sNetworkPolicy) => void;
}

export const useNetworkPolicyActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions({
        resourceType: 'networkpolicy',
        resourceLabel: 'Network Policy',
        DetailsComponent: NetworkPolicyDetails,
        detailsPropName: 'networkPolicy',
    });

    const handleDelete = createDeleteHandler(
        async (networkPolicy: any): Promise<void> => {
            await DeleteNetworkPolicy(networkPolicy.metadata.namespace, networkPolicy.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this network policy? This may affect pod network connectivity.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
