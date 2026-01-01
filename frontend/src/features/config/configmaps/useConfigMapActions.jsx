import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteConfigMap } from '../../../../wailsjs/go/main/App';
import ConfigMapDetails from '../../../components/shared/ConfigMapDetails';

export const useConfigMapActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        createDeleteHandler,
    } = useBaseResourceActions({
        resourceType: 'configmap',
        resourceLabel: 'ConfigMap',
        DetailsComponent: ConfigMapDetails,
        detailsPropName: 'configMap',
    });

    const handleDelete = createDeleteHandler(async (configMap) => {
        await DeleteConfigMap(configMap.metadata.namespace, configMap.metadata.name);
    });

    return {
        handleShowDetails,
        handleEditYaml,
        handleShowDependencies,
        handleDelete
    };
};
