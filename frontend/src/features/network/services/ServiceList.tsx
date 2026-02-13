import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import ServiceActionsMenu from './ServiceActionsMenu';
import { useServices } from '~/hooks/resources';
import { useServiceActions } from './useServiceActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteService, GetServiceYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { useMenuPosition } from '~/hooks/useMenuPosition';

// Helper to format ports display (avoids duplicate computation in render/getValue)
const getPortsDisplay = (item: any) => item.spec?.ports?.map((p: any) => `${p.port}/${p.protocol}`).join(', ') || '';

export default function ServiceList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { services, loading } = useServices(currentContext, selectedNamespaces, isVisible) as any;
    const { handleEditYaml, handleShowDependencies, handleShowDetails } = useServiceActions();
    const selection = useSelection();

    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Service',
        resourceType: 'services',
        isNamespaced: true,
        deleteApi: DeleteService,
        getYamlApi: GetServiceYaml,

    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: (item: any) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: (item: any) => item.metadata?.namespace },
        { key: 'type', label: 'Type', render: (item: any) => item.spec?.type, getValue: (item: any) => item.spec?.type },
        { key: 'clusterIP', label: 'Cluster IP', render: (item: any) => item.spec?.clusterIP, getValue: (item: any) => item.spec?.clusterIP },
        { key: 'ports', label: 'Ports', render: getPortsDisplay, getValue: getPortsDisplay },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: (item: any) => item.metadata?.creationTimestamp },
        // Hidden by default columns
        {
            key: 'externalIP',
            label: 'External IP',
            defaultHidden: true,
            render: (item: any) => {
                const ips = item.status?.loadBalancer?.ingress?.map((i: any) => i.ip || i.hostname).filter(Boolean) || [];
                return ips.length > 0 ? ips.join(', ') : <span className="text-gray-500">-</span>;
            },
            getValue: (item: any) => item.status?.loadBalancer?.ingress?.[0]?.ip || '',
        },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: (item: any) => {
                const selector = item.spec?.selector || {};
                const entries = Object.entries(selector);
                if (entries.length === 0) return '-';
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: (item: any) => Object.entries(item.spec?.selector || {}).map(([k, v]) => `${k}=${v}`).join(','),
        },
        {
            key: 'sessionAffinity',
            label: 'Session Affinity',
            defaultHidden: true,
            render: (item: any) => item.spec?.sessionAffinity || 'None',
            getValue: (item: any) => item.spec?.sessionAffinity || 'None',
        },
        {
            key: 'externalName',
            label: 'External Name',
            defaultHidden: true,
            render: (item: any) => item.spec?.externalName || <span className="text-gray-500">-</span>,
            getValue: (item: any) => item.spec?.externalName || '',
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <ServiceActionsMenu
                    service={item}
                    isOpen={activeMenuId === `service-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `service-${item.metadata.uid}`, buttonElement)}
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
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </>
    );
}
