import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteClusterRoleBinding } from 'wailsjs/go/main/App';
import ClusterRoleBindingDetails from '~/components/shared/ClusterRoleBindingDetails';
import { K8sClusterRoleBinding } from '~/types/k8s';

export interface ClusterRoleBindingActionsReturn extends BaseResourceActionsReturn<K8sClusterRoleBinding> {
    handleDelete: (clusterRoleBinding: K8sClusterRoleBinding) => void;
}

export const useClusterRoleBindingActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'clusterrolebinding',
        resourceLabel: 'ClusterRoleBinding',
        DetailsComponent: ClusterRoleBindingDetails,
        detailsPropName: 'clusterRoleBinding',
        isNamespaced: false,
    });

    const handleDelete = createDeleteHandler(
        async (clusterRoleBinding: any): Promise<void> => {
            await DeleteClusterRoleBinding(clusterRoleBinding.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this ClusterRoleBinding? Associated subjects will lose their cluster-wide permissions.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
