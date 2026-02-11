import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteClusterRole } from 'wailsjs/go/main/App';
import ClusterRoleDetails from '~/components/shared/ClusterRoleDetails';
import { K8sClusterRole } from '~/types/k8s';

export interface ClusterRoleActionsReturn extends BaseResourceActionsReturn<K8sClusterRole> {
    handleDelete: (clusterRole: K8sClusterRole) => void;
}

export const useClusterRoleActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'clusterrole',
        resourceLabel: 'ClusterRole',
        DetailsComponent: ClusterRoleDetails,
        detailsPropName: 'clusterRole',
        isNamespaced: false,
    });

    const handleDelete = createDeleteHandler(
        async (clusterRole: any): Promise<void> => {
            await DeleteClusterRole(clusterRole.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this ClusterRole? All associated permissions will be removed.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
