import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteEndpoints } from '../../../../wailsjs/go/main/App';
import EndpointsDetails from '../../../components/shared/EndpointsDetails';

export const useEndpointsActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'endpoints',
        resourceLabel: 'Endpoints',
        DetailsComponent: EndpointsDetails,
        detailsPropName: 'endpoints',
    });

    const handleDelete = createDeleteHandler(
        async (endpoints) => {
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
