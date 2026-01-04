import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import EndpointSliceActionsMenu from './EndpointSliceActionsMenu';
import { useEndpointSlices } from '../../../hooks/resources';
import { useEndpointSliceActions } from './useEndpointSliceActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useSelection } from '../../../hooks/useSelection';
import { DeleteEndpointSlice, GetEndpointSliceYaml, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import Logger from '../../../utils/Logger';

export default function EndpointSliceList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { endpointSlices, loading } = useEndpointSlices(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = useEndpointSliceActions();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const selection = useSelection();

    const [bulkActionModal, setBulkActionModal] = useState({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState({ current: 0, total: 0, status: 'idle', results: [] });

    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkActionConfirm = useCallback(async (items) => {
        Logger.info('Bulk delete started', { count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));
        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.metadata?.namespace;
            const name = item.metadata?.name;
            try {
                await DeleteEndpointSlice(namespace, name);
                results.push({ name, namespace, success: true, message: '' });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
            }
            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }
        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
    }, []);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        const entries = [];
        for (const item of items) {
            try {
                const yaml = await GetEndpointSliceYaml(item.metadata?.namespace, item.metadata?.name);
                entries.push({ namespace: item.metadata?.namespace, name: item.metadata?.name, kind: 'EndpointSlice', yaml });
            } catch (err) {
                entries.push({ namespace: item.metadata?.namespace, name: item.metadata?.name, kind: 'EndpointSlice', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `endpointslices-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
    }, []);

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            setMenuPosition({
                top: rect.bottom + 4,
                left: rect.right - 192
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId]);

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
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, handleDelete]);

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
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Delete" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />
        </>
    );
}
