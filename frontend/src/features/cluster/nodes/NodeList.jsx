import React, { useMemo } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useNodes } from '../../../hooks/useNodes';
import { useK8s } from '../../../context/K8sContext';
import { formatAge } from '../../../utils/formatting';

export default function NodeList({ isVisible }) {
    const { currentContext } = useK8s();
    const { nodes, loading } = useNodes(currentContext, isVisible);
    // Nodes are cluster-scoped, so we don't need namespace selector, but ResourceList might expect it?
    // ResourceList has showNamespaceSelector prop.

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'status', label: 'Status', render: (item) => item.status?.conditions?.find(c => c.type === 'Ready')?.status === 'True' ? 'Ready' : 'NotReady', getValue: (item) => item.status?.conditions?.find(c => c.type === 'Ready')?.status === 'True' ? 'Ready' : 'NotReady' },
        { key: 'version', label: 'Version', render: (item) => item.status?.nodeInfo?.kubeletVersion, getValue: (item) => item.status?.nodeInfo?.kubeletVersion },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
    ], []);

    return (
        <ResourceList
            title="Nodes"
            columns={columns}
            data={nodes}
            isLoading={loading}
            showNamespaceSelector={false}
            initialSort={{ key: 'name', direction: 'asc' }}
            resourceType="nodes"
        />
    );
}
