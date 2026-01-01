import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeletePV } from '../../../../wailsjs/go/main/App';
import PVDetails from '../../../components/shared/PVDetails';

export const usePVActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'pv',
        resourceLabel: 'PersistentVolume',
        DetailsComponent: PVDetails,
        detailsPropName: 'pv',
        isNamespaced: false,
    });

    const handleDelete = createDeleteHandler(async (pv) => {
        await DeletePV(pv.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
