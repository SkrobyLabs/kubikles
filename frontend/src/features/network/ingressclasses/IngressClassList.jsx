import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import IngressClassActionsMenu from './IngressClassActionsMenu';
import { useIngressClasses } from '../../../hooks/useIngressClasses';
import { useIngressClassActions } from './useIngressClassActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function IngressClassList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { ingressClasses, loading } = useIngressClasses(currentContext, isVisible);
    const { handleEditYaml, handleDelete } = useIngressClassActions();
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

    const isDefault = (ingressClass) => {
        return ingressClass.metadata?.annotations?.['ingressclass.kubernetes.io/is-default-class'] === 'true';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'controller', label: 'Controller', render: (item) => item.spec?.controller || '-', getValue: (item) => item.spec?.controller || '' },
        {
            key: 'default',
            label: 'Default',
            render: (item) => isDefault(item) ? (
                <CheckCircleIcon className="h-5 w-5 text-green-400" />
            ) : (
                <span className="text-gray-500">-</span>
            ),
            getValue: (item) => isDefault(item) ? 'Yes' : 'No'
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <IngressClassActionsMenu
                    ingressClass={item}
                    isOpen={activeMenuId === `ingressclass-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `ingressclass-${item.metadata.uid}`, buttonElement)}
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
            title="Ingress Classes"
            columns={columns}
            data={ingressClasses}
            isLoading={loading}
            showNamespaceSelector={false}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="ingressclasses"
        />
    );
}
