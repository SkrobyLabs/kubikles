import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useConfigMaps } from '../../../hooks/useConfigMaps';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import ConfigMapActionsMenu from './ConfigMapActionsMenu';
import { useConfigMapActions } from './useConfigMapActions';

export default function ConfigMapList({ isVisible }) {
    const { currentContext, currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { configMaps, loading } = useConfigMaps(currentContext, currentNamespace, isVisible);
    const { handleEditYaml, handleDelete } = useConfigMapActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'keys', label: 'Keys', render: (item) => Object.keys(item.data || {}).join(', '), getValue: (item) => Object.keys(item.data || {}).join(', ') },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: '',
            render: (item) => (
                <ConfigMapActionsMenu
                    configMap={item}
                    isOpen={activeMenuId === `configmap-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `configmap-${item.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => ''
        }
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleDelete]);

    return (
        <ResourceList
            title="ConfigMaps"
            columns={columns}
            data={configMaps}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={currentNamespace}
            onNamespaceChange={setCurrentNamespace}
            showNamespaceSelector={true}
        />
    );
}
