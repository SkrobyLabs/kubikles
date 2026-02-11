import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteRoleBinding } from 'wailsjs/go/main/App';
import RoleBindingDetails from '~/components/shared/RoleBindingDetails';
import { K8sRoleBinding } from '~/types/k8s';

export interface RoleBindingActionsReturn extends BaseResourceActionsReturn<K8sRoleBinding> {
    handleDelete: (roleBinding: K8sRoleBinding) => void;
}

export const useRoleBindingActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'rolebinding',
        resourceLabel: 'RoleBinding',
        DetailsComponent: RoleBindingDetails,
        detailsPropName: 'roleBinding',
    });

    const handleDelete = createDeleteHandler(
        async (roleBinding: any): Promise<void> => {
            await DeleteRoleBinding(roleBinding.metadata.namespace, roleBinding.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this RoleBinding? Associated subjects will lose their permissions.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
