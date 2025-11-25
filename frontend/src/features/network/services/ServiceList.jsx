import React, { useMemo } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import ServiceActionsMenu from './ServiceActionsMenu';
import { useServices } from '../../../hooks/useServices';
import { useServiceActions } from './useServiceActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function ServiceList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { services, loading } = useServices(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleShowDependencies } = useServiceActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'type', label: 'Type', render: (item) => item.spec?.type, getValue: (item) => item.spec?.type },
        { key: 'clusterIP', label: 'Cluster IP', render: (item) => item.spec?.clusterIP, getValue: (item) => item.spec?.clusterIP },
        { key: 'ports', label: 'Ports', render: (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '', getValue: (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '' },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ServiceActionsMenu
                    service={item}
                    isOpen={activeMenuId === `service-${item.metadata.uid}`}
                    onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `service-${item.metadata.uid}` : null)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, setActiveMenuId, handleEditYaml, handleShowDependencies]);

    return (
        <ResourceList
            title="Services"
            columns={columns}
            data={services}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'asc' }}
            resourceType="services"
        />
    );
}
