import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import EndpointSliceActionsMenu from './EndpointSliceActionsMenu';
import { useEndpointSlices } from '../../../hooks/resources';
import { useEndpointSliceActions } from './useEndpointSliceActions';
import { useK8s } from '../../../context';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteEndpointSlice, GetEndpointSliceYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function EndpointSliceList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { endpointSlices, loading } = useEndpointSlices(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useEndpointSliceActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'EndpointSlice',
        resourceType: 'endpointslices',
        isNamespaced: true,
        deleteApi: DeleteEndpointSlice,
        getYamlApi: GetEndpointSliceYaml,
        currentContext,
    });

    const getEndpointCount = (eps) => {
        return (eps.endpoints || []).length;
    };

    const getReadyCount = (eps) => {
        return (eps.endpoints || []).filter(ep => ep.conditions?.ready === true).length;
    };

    const getPorts = (eps) => {
        const ports = eps.ports || [];
        if (ports.length === 0) return '-';
        return ports.slice(0, 3).map(p => `${p.port}/${p.protocol || 'TCP'}`).join(', ') +
            (ports.length > 3 ? ` +${ports.length - 3}` : '');
    };

    const getServiceName = (eps) => {
        return eps.metadata?.labels?.['kubernetes.io/service-name'] || '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'service', label: 'Service', render: (item) => getServiceName(item), getValue: (item) => getServiceName(item) },
        { key: 'addressType', label: 'Type', render: (item) => item.addressType || '-', getValue: (item) => item.addressType || '' },
        {
            key: 'endpoints',
            label: 'Endpoints',
            render: (item) => {
                const ready = getReadyCount(item);
                const total = getEndpointCount(item);
                return `${ready}/${total}`;
            },
            getValue: (item) => getEndpointCount(item)
        },
        { key: 'ports', label: 'Ports', render: (item) => getPorts(item), getValue: (item) => getPorts(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <EndpointSliceActionsMenu
                    endpointSlice={item}
                    isOpen={activeMenuId === `endpointslice-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `endpointslice-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(slice) => openBulkDelete([slice])}
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
                title="Endpoint Slices"
                columns={columns}
                data={endpointSlices}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="endpointslices"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
