import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteMutatingWebhookConfiguration } from 'wailsjs/go/main/App';
import MutatingWebhookDetails from '~/components/shared/MutatingWebhookDetails';
import { K8sMutatingWebhookConfiguration } from '~/types/k8s';

interface MutatingWebhookActionsReturn extends BaseResourceActionsReturn<K8sMutatingWebhookConfiguration> {
    handleDelete: (webhook: K8sMutatingWebhookConfiguration) => void;
}

export const useMutatingWebhookActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions<K8sMutatingWebhookConfiguration>({
        resourceType: 'mutatingwebhookconfiguration',
        resourceLabel: 'Mutating Webhook Configuration',
        DetailsComponent: MutatingWebhookDetails,
        detailsPropName: 'webhook',
    });

    const handleDelete = createDeleteHandler(
        async (webhook: any): Promise<void> => {
            await DeleteMutatingWebhookConfiguration(webhook.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this mutating webhook configuration? This may affect resource mutations in the cluster.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
