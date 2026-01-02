import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteValidatingWebhookConfiguration } from '../../../../wailsjs/go/main/App';
import ValidatingWebhookDetails from '../../../components/shared/ValidatingWebhookDetails';

export const useValidatingWebhookActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'validatingwebhookconfiguration',
        resourceLabel: 'Validating Webhook Configuration',
        DetailsComponent: ValidatingWebhookDetails,
        detailsPropName: 'webhook',
    });

    const handleDelete = createDeleteHandler(
        async (webhook) => {
            await DeleteValidatingWebhookConfiguration(currentContext, webhook.metadata.name);
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
