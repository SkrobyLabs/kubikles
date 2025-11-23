import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useConfigMaps } from '../../../hooks/useConfigMaps';
import { useK8s } from '../../../context/K8sContext';
import { formatAge } from '../../../utils/formatting';

export default function ConfigMapList({ isVisible }) {
    const { currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { configMaps, loading } = useConfigMaps(currentNamespace, isVisible);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'keys', label: 'Keys', render: (item) => Object.keys(item.data || {}).join(', '), getValue: (item) => Object.keys(item.data || {}).join(', ') },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
    ], []);

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
