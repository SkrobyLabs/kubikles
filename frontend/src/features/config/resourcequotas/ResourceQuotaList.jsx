import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import ResourceQuotaActionsMenu from './ResourceQuotaActionsMenu';
import { useResourceQuotas } from '../../../hooks/resources';
import { useResourceQuotaActions } from './useResourceQuotaActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function ResourceQuotaList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { resourceQuotas, loading } = useResourceQuotas(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleDelete } = useResourceQuotaActions();
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

    const getResourceCount = (quota) => {
        return Object.keys(quota.spec?.hard || {}).length;
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'resources', label: 'Resources', render: (item) => getResourceCount(item), getValue: (item) => getResourceCount(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ResourceQuotaActionsMenu
                    resourceQuota={item}
                    isOpen={activeMenuId === `resourcequota-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `resourcequota-${item.metadata.uid}`, buttonElement)}
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
            title="Resource Quotas"
            columns={columns}
            data={resourceQuotas}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="resourcequotas"
            onRowClick={handleShowDetails}
        />
    );
}
