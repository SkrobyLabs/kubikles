import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import ServiceActionsMenu from './ServiceActionsMenu';
import { useServices } from '~/hooks/resources';
import { useServiceActions } from './useServiceActions';
import { useK8s } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteService, GetServiceYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';

import { serviceSemantics, servicePortsDisplay, serviceLoadBalancerAddresses } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildServiceViewColumns, SERVICE_HIDDEN_BY_DEFAULT, SERVICE_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const SERVICE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const SERVICE_VIEW_OPTIONS = { sort: SERVICE_INITIAL_SORT, columns: defaultViewColumns('services', SERVICE_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function ServiceList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { services, loading, loadState, view } = useServices(currentContext, selectedNamespaces, isVisible, SERVICE_VIEW_OPTIONS) as any;
    const { handleEditYaml, handleShowDependencies, handleShowDetails } = useStableCallbacks(useServiceActions());
    const selection = useSelection();
    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

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
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: serviceSemantics.name.getValue },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: serviceSemantics.namespace.getValue },
        { key: 'type', label: 'Type', render: (item: any) => item.spec?.type, getValue: serviceSemantics.type.getValue },
        { key: 'clusterIP', label: 'Cluster IP', render: (item: any) => item.spec?.clusterIP, getValue: serviceSemantics.clusterIP.getValue },
        { key: 'ports', label: 'Ports', render: servicePortsDisplay, getValue: serviceSemantics.ports.getValue },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: serviceSemantics.age.getValue },
        // Hidden by default columns
        {
            key: 'externalIP',
            label: 'External IP',
            defaultHidden: true,
            render: (item: any) => {
                const ips = serviceLoadBalancerAddresses(item);
                return ips.length > 0 ? ips.join(', ') : <span className="text-gray-500">-</span>;
            },
            getValue: serviceSemantics.externalIP.getValue,
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
            getValue: serviceSemantics.selector.getValue,
        },
        {
            key: 'sessionAffinity',
            label: 'Session Affinity',
            defaultHidden: true,
            render: (item: any) => item.spec?.sessionAffinity || 'None',
            getValue: serviceSemantics.sessionAffinity.getValue,
        },
        {
            key: 'externalName',
            label: 'External Name',
            defaultHidden: true,
            render: (item: any) => item.spec?.externalName || <span className="text-gray-500">-</span>,
            getValue: serviceSemantics.externalName.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={ServiceActionsMenu}
                    menuId={`service-${item.metadata.uid}`}
                    service={item}
                    onEditYaml={handleEditYaml}
                    onShowDependencies={handleShowDependencies}
                    onDelete={() => openBulkDelete([item])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [handleEditYaml, handleShowDependencies, openBulkDelete]);

    const viewColumns = useMemo(
        () => buildServiceViewColumns({
            onEditYaml: handleEditYaml as (item: any) => void,
            onShowDependencies: handleShowDependencies as (item: any) => void,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, handleShowDependencies, openBulkDelete]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Services"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={SERVICE_PINNED_COLUMNS}
                data={view ? NO_DATA : services}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={SERVICE_INITIAL_SORT}
                resourceType="services"
                onRowClick={view ? showDetails : handleShowDetails}
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
