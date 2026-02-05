import { useBaseResourceActions, BaseResourceActionsReturn } from '../../../hooks/useBaseResourceActions';
import { DeleteCSIDriver } from '../../../../wailsjs/go/main/App';
import CSIDriverDetails from '../../../components/shared/CSIDriverDetails';
import { K8sCSIDriver } from '../../../types/k8s';

export interface CSIDriverActionsReturn extends BaseResourceActionsReturn<K8sCSIDriver> {
    handleDelete: (csiDriver: K8sCSIDriver) => void;
}

export const useCSIDriverActions = (): CSIDriverActionsReturn => {
    const {
        handleShowDetails,
        handleEditYaml,
        createDeleteHandler,
    } = useBaseResourceActions<K8sCSIDriver>({
        resourceType: 'csidriver',
        resourceLabel: 'CSIDriver',
        DetailsComponent: CSIDriverDetails,
        detailsPropName: 'csiDriver',
    });

    const handleDelete = createDeleteHandler(
        async (csiDriver: K8sCSIDriver): Promise<void> => {
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
