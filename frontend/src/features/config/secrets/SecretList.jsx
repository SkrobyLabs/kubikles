import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useSecrets } from '../../../hooks/useSecrets';
import { useK8s } from '../../../context/K8sContext';
import { formatAge } from '../../../utils/formatting';

export default function SecretList({ isVisible }) {
    const { currentNamespace, setCurrentNamespace, namespaces } = useK8s();
    const { secrets, loading } = useSecrets(currentNamespace, isVisible);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'type', label: 'Type', render: (item) => item.type, getValue: (item) => item.type },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
    ], []);

    return (
        <ResourceList
            title="Secrets"
            columns={columns}
            data={secrets}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={currentNamespace}
            onNamespaceChange={setCurrentNamespace}
            showNamespaceSelector={true}
        />
    );
}
