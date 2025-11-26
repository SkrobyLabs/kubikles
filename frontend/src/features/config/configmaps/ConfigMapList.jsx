import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useConfigMaps } from '../../../hooks/useConfigMaps';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ConfigMapActionsMenu from './ConfigMapActionsMenu';
import { useConfigMapActions } from './useConfigMapActions';

export default function ConfigMapList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { configMaps, loading } = useConfigMaps(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleShowDependencies, handleDelete } = useConfigMapActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'keys', label: 'Keys', render: (item) => Object.keys(item.data || {}).join(', '), getValue: (item) => Object.keys(item.data || {}).join(', ') },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ConfigMapActionsMenu
                    configMap={item}
                    isOpen={activeMenuId === `configmap-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `configmap-${item.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleShowDependencies, handleDelete]);

    return (
        <ResourceList
            title="ConfigMaps"
            columns={columns}
            data={configMaps}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="configmaps"
        />
    );
}
