import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useServices } from '../../../hooks/useServices';
import { useK8s } from '../../../context/K8sContext';
import { formatAge } from '../../../utils/formatting';

export default function ServiceList({ isVisible }) {
    const { currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { services, loading } = useServices(currentNamespace, isVisible);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'type', label: 'Type', render: (item) => item.spec?.type, getValue: (item) => item.spec?.type },
        { key: 'clusterIP', label: 'Cluster IP', render: (item) => item.spec?.clusterIP, getValue: (item) => item.spec?.clusterIP },
        { key: 'ports', label: 'Ports', render: (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '', getValue: (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '' },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
    ], []);

    return (
        <ResourceList
            title="Services"
            columns={columns}
            data={services}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={currentNamespace}
            onNamespaceChange={setCurrentNamespace}
            showNamespaceSelector={true}
        />
    );
}
