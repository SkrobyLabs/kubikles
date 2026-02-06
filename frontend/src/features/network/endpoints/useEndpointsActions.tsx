import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteEndpoints } from 'wailsjs/go/main/App';
import EndpointsDetails from '~/components/shared/EndpointsDetails';
import { K8sEndpoints } from '~/types/k8s';

interface EndpointsActionsReturn extends BaseResourceActionsReturn<K8sEndpoints> {
    handleDelete: (endpoints: K8sEndpoints) => void;
}

export const useEndpointsActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions({
        resourceType: 'endpoints',
        resourceLabel: 'Endpoints',
        DetailsComponent: EndpointsDetails,
        detailsPropName: 'endpoints',
    });

    const handleDelete = createDeleteHandler(
        async (endpoints: any): Promise<void> => {
            await DeleteEndpoints(endpoints.metadata.namespace, endpoints.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete these endpoints? This may affect service connectivity.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
