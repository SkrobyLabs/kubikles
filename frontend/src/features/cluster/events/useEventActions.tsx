import { useBaseResourceActions, BaseResourceActionsReturn } from '~/hooks/useBaseResourceActions';
import { DeleteEvent } from 'wailsjs/go/main/App';
import EventDetails from '~/components/shared/EventDetails';
import { K8sEvent } from '~/types/k8s';

export interface EventActionsReturn extends BaseResourceActionsReturn<K8sEvent> {
    handleDelete: (event: K8sEvent) => void;
}

export const useEventActions = (): EventActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
    } = useBaseResourceActions<K8sEvent>({
        resourceType: 'event',
        resourceLabel: 'Event',
        DetailsComponent: EventDetails,
        detailsPropName: 'event',
        hasDependencies: false,
        getTabTitle: (event: K8sEvent) => event.reason || 'Event',
    });

    const handleDelete = createDeleteHandler(async (event: K8sEvent): Promise<void> => {
        await DeleteEvent(event.metadata.namespace!, event.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
