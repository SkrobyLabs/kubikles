import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import LeaseActionsMenu from './LeaseActionsMenu';
import { useLeases } from '../../../hooks/resources';
import { useLeaseActions } from './useLeaseActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function LeaseList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { leases, loading } = useLeases(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = useLeaseActions();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });

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

    const getHolderIdentity = (lease) => {
        return lease.spec?.holderIdentity || '-';
    };

    const getLeaseDuration = (lease) => {
        const duration = lease.spec?.leaseDurationSeconds;
        if (!duration) return '-';
        return `${duration}s`;
    };

    const getRenewTime = (lease) => {
        const renewTime = lease.spec?.renewTime;
        if (!renewTime) return '-';
        return formatAge(renewTime);
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'holder', label: 'Holder Identity', render: (item) => getHolderIdentity(item), getValue: (item) => getHolderIdentity(item) },
        { key: 'duration', label: 'Duration', render: (item) => getLeaseDuration(item), getValue: (item) => item.spec?.leaseDurationSeconds || 0 },
        { key: 'renewTime', label: 'Last Renewed', render: (item) => getRenewTime(item), getValue: (item) => item.spec?.renewTime || '' },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <LeaseActionsMenu
                    lease={item}
                    isOpen={activeMenuId === `lease-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `lease-${item.metadata.uid}`, buttonElement)}
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
        <ResourceList
            title="Leases"
            columns={columns}
            data={leases}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="leases"
            onRowClick={handleShowDetails}
        />
    );
}
