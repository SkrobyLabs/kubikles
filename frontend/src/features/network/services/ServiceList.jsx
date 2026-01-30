import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import ServiceActionsMenu from './ServiceActionsMenu';
import { useServices } from '../../../hooks/resources';
import { useServiceActions } from './useServiceActions';
import { useK8s } from '../../../context/K8sContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteService, GetServiceYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

// Helper to format ports display (avoids duplicate computation in render/getValue)
const getPortsDisplay = (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '';

export default function ServiceList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { services, loading } = useServices(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleShowDependencies, handleShowDetails } = useServiceActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Service',
        resourceType: 'services',
        isNamespaced: true,
        deleteApi: DeleteService,
        getYamlApi: GetServiceYaml,
        currentContext,
    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'type', label: 'Type', render: (item) => item.spec?.type, getValue: (item) => item.spec?.type },
        { key: 'clusterIP', label: 'Cluster IP', render: (item) => item.spec?.clusterIP, getValue: (item) => item.spec?.clusterIP },
        { key: 'ports', label: 'Ports', render: getPortsDisplay, getValue: getPortsDisplay },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ServiceActionsMenu
                    service={item}
                    isOpen={activeMenuId === `service-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `service-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={() => openBulkDelete([item])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
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
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="services"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
