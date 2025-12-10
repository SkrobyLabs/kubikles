import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import NamespaceActionsMenu from './NamespaceActionsMenu';
import { useNamespacesList } from '../../../hooks/useNamespacesList';
import { useNamespaceActions } from './useNamespaceActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

// Get namespace status from conditions
function getNamespaceStatus(namespace) {
    const phase = namespace.status?.phase;
    if (phase === 'Active') return 'Active';
    if (phase === 'Terminating') return 'Terminating';
    return phase || 'Unknown';
}

function getStatusColor(status) {
    switch (status) {
        case 'Active':
            return 'text-green-400';
        case 'Terminating':
            return 'text-yellow-400';
        default:
            return 'text-gray-400';
    }
}

export default function NamespaceList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
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
    const { namespaces, loading } = useNamespacesList(currentContext, isVisible);
    const { handleEditYaml, handleDelete } = useNamespaceActions();

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item) => item.metadata?.name,
            getValue: (item) => item.metadata?.name
        },
        {
            key: 'status',
            label: 'Status',
            render: (item) => {
                const status = getNamespaceStatus(item);
                return <span className={getStatusColor(status)}>{status}</span>;
            },
            getValue: (item) => getNamespaceStatus(item)
        },
        {
            key: 'age',
            label: 'Age',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <NamespaceActionsMenu
                    namespace={item}
                    isOpen={activeMenuId === `namespace-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `namespace-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onDelete={() => handleDelete(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleDelete]);

    return (
        <ResourceList
            title="Namespaces"
            columns={columns}
            data={namespaces}
            isLoading={loading}
            showNamespaceSelector={false}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="namespaces"
        />
    );
}
