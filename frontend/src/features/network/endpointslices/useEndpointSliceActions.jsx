import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteEndpointSlice } from '../../../../wailsjs/go/main/App';
import EndpointSliceDetails from '../../../components/shared/EndpointSliceDetails';

export const useEndpointSliceActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'endpointslices',
        resourceLabel: 'EndpointSlice',
        DetailsComponent: EndpointSliceDetails,
        detailsPropName: 'endpointSlice',
    });

    const handleDelete = createDeleteHandler(
        async (endpointSlice) => {
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
