import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteRole } from 'wailsjs/go/main/App';
import RoleDetails from '~/components/shared/RoleDetails';
import { K8sRole } from '~/types/k8s';

export interface RoleActionsReturn extends BaseResourceActionsReturn<K8sRole> {
    handleDelete: (role: K8sRole) => void;
}

export const useRoleActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'role',
        resourceLabel: 'Role',
        DetailsComponent: RoleDetails,
        detailsPropName: 'role',
    });

    const handleDelete = createDeleteHandler(
        async (role: any): Promise<void> => {
            await DeleteRole(role.metadata.namespace, role.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this Role? All associated permissions will be removed.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
