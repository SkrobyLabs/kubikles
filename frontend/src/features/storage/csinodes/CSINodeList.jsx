import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import CSINodeActionsMenu from './CSINodeActionsMenu';
import { useCSINodes } from '../../../hooks/resources';
import { useCSINodeActions } from './useCSINodeActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function CSINodeList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { csiNodes, loading } = useCSINodes(currentContext, isVisible);
    const { handleShowDetails, handleEditYaml, handleDelete } = useCSINodeActions();
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

    const getDriverCount = (csiNode) => {
        const drivers = csiNode.spec?.drivers || [];
        return drivers.length;
    };

    const getDriverNames = (csiNode) => {
        const drivers = csiNode.spec?.drivers || [];
        if (drivers.length === 0) return '-';
        if (drivers.length <= 2) {
            return drivers.map(d => d.name).join(', ');
        }
        return `${drivers[0].name}, +${drivers.length - 1} more`;
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'driverCount',
            label: 'Drivers',
            render: (item) => (
                <span className={getDriverCount(item) > 0 ? 'text-green-400' : 'text-gray-500'}>
                    {getDriverCount(item)}
                </span>
            ),
            getValue: (item) => getDriverCount(item)
        },
        { key: 'driverNames', label: 'Driver Names', render: (item) => getDriverNames(item), getValue: (item) => getDriverNames(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <CSINodeActionsMenu
                    csiNode={item}
                    isOpen={activeMenuId === `csinode-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `csinode-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleDelete]);

    return (
        <ResourceList
            title="CSI Nodes"
            columns={columns}
            data={csiNodes}
            isLoading={loading}
            showNamespaceSelector={false}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="csinodes"
            onRowClick={handleShowDetails}
        />
    );
}
