import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteCSIDriver } from '../../../../wailsjs/go/main/App';
import CSIDriverDetails from '../../../components/shared/CSIDriverDetails';

export const useCSIDriverActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'csidriver',
        resourceLabel: 'CSIDriver',
        DetailsComponent: CSIDriverDetails,
        detailsPropName: 'csiDriver',
    });

    const handleDelete = createDeleteHandler(
        async (csiDriver) => {
            await DeleteCSIDriver(csiDriver.metadata.name);
        },
        { confirmMessage: 'Are you sure you want to delete this CSI Driver? Storage provisioning may be affected.' }
    );

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
