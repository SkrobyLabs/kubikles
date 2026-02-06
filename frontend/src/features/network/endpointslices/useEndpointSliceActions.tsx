import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteEndpointSlice } from 'wailsjs/go/main/App';
import EndpointSliceDetails from '~/components/shared/EndpointSliceDetails';
import { K8sEndpointSlice } from '~/types/k8s';

interface EndpointSliceActionsReturn extends BaseResourceActionsReturn<K8sEndpointSlice> {
    handleDelete: (endpointSlice: K8sEndpointSlice) => void;
}

export const useEndpointSliceActions = (): EndpointSliceActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions({
        resourceType: 'endpointslices',
        resourceLabel: 'EndpointSlice',
        DetailsComponent: EndpointSliceDetails,
        detailsPropName: 'endpointSlice',
    });

    const handleDelete = createDeleteHandler(
        async (endpointSlice: K8sEndpointSlice): Promise<void> => {
            await DeleteEndpointSlice(endpointSlice.metadata.namespace, endpointSlice.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this EndpointSlice? This may affect service connectivity.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
