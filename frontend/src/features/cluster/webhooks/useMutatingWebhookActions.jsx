import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteMutatingWebhookConfiguration } from '../../../../wailsjs/go/main/App';
import MutatingWebhookDetails from '../../../components/shared/MutatingWebhookDetails';

export const useMutatingWebhookActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
        currentContext,
    } = useBaseResourceActions({
        resourceType: 'mutatingwebhookconfiguration',
        resourceLabel: 'Mutating Webhook Configuration',
        DetailsComponent: MutatingWebhookDetails,
        detailsPropName: 'webhook',
    });

    const handleDelete = createDeleteHandler(
        async (webhook) => {
            await DeleteMutatingWebhookConfiguration(currentContext, webhook.metadata.name);
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
