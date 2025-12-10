import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { usePVCs } from '../../../hooks/usePVCs';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import PVCActionsMenu from './PVCActionsMenu';
import { usePVCActions } from './usePVCActions';

const getStatusColor = (phase) => {
    switch (phase) {
        case 'Bound':
            return 'text-green-400';
        case 'Pending':
            return 'text-yellow-400';
        case 'Lost':
            return 'text-red-400';
        default:
            return 'text-gray-400';
    }
};

export default function PVCList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { pvcs, loading } = usePVCs(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleShowDependencies, handleDelete } = usePVCActions();
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

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'status',
            label: 'Status',
            render: (item) => (
                <span className={getStatusColor(item.status?.phase)}>
                    {item.status?.phase || 'Unknown'}
                </span>
            ),
            getValue: (item) => item.status?.phase
        },
        { key: 'volume', label: 'Volume', render: (item) => item.spec?.volumeName || '-', getValue: (item) => item.spec?.volumeName || '' },
        {
            key: 'capacity',
            label: 'Capacity',
            render: (item) => item.status?.capacity?.storage || item.spec?.resources?.requests?.storage || '-',
            getValue: (item) => item.status?.capacity?.storage || item.spec?.resources?.requests?.storage || ''
        },
        { key: 'accessModes', label: 'Access Modes', render: (item) => item.spec?.accessModes?.join(', ') || '-', getValue: (item) => item.spec?.accessModes?.join(', ') || '' },
        { key: 'storageClass', label: 'Storage Class', render: (item) => item.spec?.storageClassName || '-', getValue: (item) => item.spec?.storageClassName || '' },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PVCActionsMenu
                    pvc={item}
                    isOpen={activeMenuId === `pvc-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `pvc-${item.metadata.uid}`, buttonElement)}
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
            title="Persistent Volume Claims"
            columns={columns}
            data={pvcs}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="pvcs"
        />
    );
}
