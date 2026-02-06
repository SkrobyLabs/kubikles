import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import EndpointsActionsMenu from './EndpointsActionsMenu';
import { useEndpoints } from '~/hooks/resources';
import { useEndpointsActions } from './useEndpointsActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteEndpoints, GetEndpointsYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

export default function EndpointsList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { endpoints, loading } = useEndpoints(currentContext, selectedNamespaces, isVisible) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies } = useEndpointsActions();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Endpoints',
        resourceType: 'endpoints',
        isNamespaced: true,
        deleteApi: DeleteEndpoints,
        getYamlApi: GetEndpointsYaml,

    });

    const getAddressCount = (ep: any) => {
        let count = 0;
        (ep.subsets || []).forEach((subset: any) => {
            count += (subset.addresses || []).length;
            count += (subset.notReadyAddresses || []).length;
        });
        return count;
    };

    const getReadyCount = (ep: any) => {
        let count = 0;
        (ep.subsets || []).forEach((subset: any) => {
            count += (subset.addresses || []).length;
        });
        return count;
    };

    const getPorts = (ep: any) => {
        const ports = new Set<any>();
        (ep.subsets || []).forEach((subset: any) => {
            (subset.ports || []).forEach((port: any) => {
                ports.add(`${port.port}/${port.protocol || 'TCP'}`);
            });
        });
        return ports.size > 0 ? Array.from(ports).slice(0, 3).join(', ') + (ports.size > 3 ? ` +${ports.size - 3}` : '') : '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        {
            key: 'endpoints',
            label: 'Endpoints',
            render: (item: any) => {
                const ready = getReadyCount(item);
                const total = getAddressCount(item);
                return `${ready}/${total}`;
            },
            getValue: (item: any) => getAddressCount(item)
        },
        { key: 'ports', label: 'Ports', render: (item: any) => getPorts(item), getValue: (item: any) => getPorts(item) },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <EndpointsActionsMenu
                    endpoints={item}
                    isOpen={activeMenuId === `endpoints-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `endpoints-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={(ep: any) => openBulkDelete([ep])}
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
                title="Endpoints"
                columns={columns}
                data={endpoints}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="endpoints"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action || ''} actionLabel="Delete" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={exportYaml} progress={bulkProgress} />
        </>
    );
}
