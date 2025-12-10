import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import IngressActionsMenu from './IngressActionsMenu';
import { useIngresses } from '../../../hooks/useIngresses';
import { useIngressActions } from './useIngressActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';

export default function IngressList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { ingresses, loading } = useIngresses(currentContext, selectedNamespaces, isVisible);
    const { handleEditYaml, handleShowDependencies, handleDelete } = useIngressActions();
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

    const getHosts = (ingress) => {
        const rules = ingress.spec?.rules || [];
        const hosts = rules.map(r => r.host).filter(Boolean);
        return hosts.length > 0 ? hosts.join(', ') : '*';
    };

    const getPaths = (ingress) => {
        const rules = ingress.spec?.rules || [];
        const paths = [];
        for (const rule of rules) {
            const httpPaths = rule.http?.paths || [];
            for (const p of httpPaths) {
                paths.push(p.path || '/');
            }
        }
        return paths.length > 0 ? paths.slice(0, 3).join(', ') + (paths.length > 3 ? '...' : '') : '-';
    };

    const getIngressClass = (ingress) => {
        return ingress.spec?.ingressClassName || ingress.metadata?.annotations?.['kubernetes.io/ingress.class'] || '-';
    };

    const getAddress = (ingress) => {
        const lbIngress = ingress.status?.loadBalancer?.ingress || [];
        if (lbIngress.length === 0) return '-';
        const addresses = lbIngress.map(lb => lb.ip || lb.hostname).filter(Boolean);
        return addresses.length > 0 ? addresses.join(', ') : '-';
    };

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        { key: 'class', label: 'Class', render: (item) => getIngressClass(item), getValue: (item) => getIngressClass(item) },
        { key: 'hosts', label: 'Hosts', render: (item) => getHosts(item), getValue: (item) => getHosts(item) },
        { key: 'address', label: 'Address', render: (item) => getAddress(item), getValue: (item) => getAddress(item) },
        { key: 'paths', label: 'Paths', render: (item) => getPaths(item), getValue: (item) => getPaths(item) },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <IngressActionsMenu
                    ingress={item}
                    isOpen={activeMenuId === `ingress-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `ingress-${item.metadata.uid}`, buttonElement)}
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
            title="Ingresses"
            columns={columns}
            data={ingresses}
            isLoading={loading}
            namespaces={namespaces}
            currentNamespace={selectedNamespaces}
            onNamespaceChange={setSelectedNamespaces}
            showNamespaceSelector={true}
            multiSelectNamespaces={true}
            highlightedUid={activeMenuId}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType="ingresses"
        />
    );
}
