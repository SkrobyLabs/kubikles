import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteIngress } from 'wailsjs/go/main/App';
import IngressDetails from '~/components/shared/IngressDetails';
import { K8sIngress } from '~/types/k8s';

export const useIngressActions = (): BaseResourceActionsReturn<K8sIngress> => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions<K8sIngress>({
        resourceType: 'ingress',
        resourceLabel: 'Ingress',
        DetailsComponent: IngressDetails,
        detailsPropName: 'ingress',
    });

    const handleDelete = createDeleteHandler(async (ingress: K8sIngress): Promise<void> => {
        await DeleteIngress(ingress.metadata.namespace, ingress.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
