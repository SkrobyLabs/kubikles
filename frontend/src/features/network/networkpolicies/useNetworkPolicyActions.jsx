import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteNetworkPolicy } from '../../../../wailsjs/go/main/App';
import NetworkPolicyDetails from '../../../components/shared/NetworkPolicyDetails';

export const useNetworkPolicyActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'networkpolicy',
        resourceLabel: 'Network Policy',
        DetailsComponent: NetworkPolicyDetails,
        detailsPropName: 'networkPolicy',
    });

    const handleDelete = createDeleteHandler(
        async (networkPolicy) => {
            await DeleteNetworkPolicy(currentContext, networkPolicy.metadata.namespace, networkPolicy.metadata.name);
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
