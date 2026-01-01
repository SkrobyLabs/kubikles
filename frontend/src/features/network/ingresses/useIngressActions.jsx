import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteIngress } from '../../../../wailsjs/go/main/App';
import IngressDetails from '../../../components/shared/IngressDetails';

export const useIngressActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'ingress',
        resourceLabel: 'Ingress',
        DetailsComponent: IngressDetails,
        detailsPropName: 'ingress',
    });

    const handleDelete = createDeleteHandler(async (ingress) => {
        await DeleteIngress(ingress.metadata.namespace, ingress.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
