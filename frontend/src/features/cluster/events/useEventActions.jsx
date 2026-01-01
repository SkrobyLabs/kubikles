import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteEvent } from '../../../../wailsjs/go/main/App';
import EventDetails from '../../../components/shared/EventDetails';

export const useEventActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'event',
        resourceLabel: 'Event',
        DetailsComponent: EventDetails,
        detailsPropName: 'event',
        hasDependencies: false,
        getTabTitle: (event) => event.reason || 'Event',
    });

    const handleDelete = createDeleteHandler(async (event) => {
        await DeleteEvent(event.metadata.namespace, event.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
