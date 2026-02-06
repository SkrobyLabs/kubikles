import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteValidatingWebhookConfiguration } from 'wailsjs/go/main/App';
import ValidatingWebhookDetails from '~/components/shared/ValidatingWebhookDetails';
import { K8sValidatingWebhookConfiguration } from '~/types/k8s';

interface ValidatingWebhookActionsReturn extends BaseResourceActionsReturn<K8sValidatingWebhookConfiguration> {
    handleDelete: (webhook: K8sValidatingWebhookConfiguration) => void;
}

export const useValidatingWebhookActions = (): any => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,

    } = useBaseResourceActions<K8sValidatingWebhookConfiguration>({
        resourceType: 'validatingwebhookconfiguration',
        resourceLabel: 'Validating Webhook Configuration',
        DetailsComponent: ValidatingWebhookDetails,
        detailsPropName: 'webhook',
    });

    const handleDelete = createDeleteHandler(
        async (webhook: any): Promise<void> => {
            await DeleteValidatingWebhookConfiguration(webhook.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this validating webhook configuration? This may affect resource validation in the cluster.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
