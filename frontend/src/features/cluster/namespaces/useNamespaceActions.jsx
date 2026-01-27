import { useBaseResourceActions } from '../../../hooks/useBaseResourceActions';
import { DeleteNamespace } from '../../../../wailsjs/go/main/App';
import NamespaceDetails from '../../../components/shared/NamespaceDetails';
import Logger from '../../../utils/Logger';

export const useNamespaceActions = () => {
    const {
        handleShowDetails,
        handleEditYaml,
        openModal,
        closeModal,
        addNotification,
    } = useBaseResourceActions({
        resourceType: 'namespace',
        resourceLabel: 'Namespace',
        DetailsComponent: NamespaceDetails,
        detailsPropName: 'namespace',
        isNamespaced: false,
        hasDependencies: false,
    });

    // Custom delete handler for namespace-specific warning
    const handleDelete = (namespace) => {
        const name = namespace.metadata.name;
        Logger.info("Delete Namespace requested", { name });

        const systemNamespaces = ['default', 'kube-system', 'kube-public', 'kube-node-lease'];
        const isSystemNamespace = systemNamespaces.includes(name);

        openModal({
            title: `Delete Namespace ${name}?`,
            content: isSystemNamespace
                ? `WARNING: "${name}" is a system namespace. Deleting it may cause cluster issues!\n\nThis will delete ALL resources within this namespace!`
                : `Are you sure you want to delete namespace "${name}"?\n\nThis will delete ALL resources within this namespace!`,
            confirmText: 'Delete',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await DeleteNamespace(name);
                    Logger.info("Namespace deleted successfully", { name });
                    closeModal();
                } catch (err) {
                    Logger.error("Failed to delete namespace", err);
                    addNotification({ type: 'error', title: 'Failed to delete namespace', message: String(err) });
                }
            }
        });
    };

    return {
        handleShowDetails,
        handleEditYaml,
        handleDelete
    };
};
