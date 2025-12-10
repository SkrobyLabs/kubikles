import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import { useCustomResources } from '../../../hooks/useCustomResources';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import CustomResourceActionsMenu from './CustomResourceActionsMenu';
import { useCustomResourceActions } from './useCustomResourceActions';

/**
 * Generic list component for custom resource instances
 * @param {Object} props
 * @param {Object} props.crdInfo - CRD information: { group, version, resource, kind, namespaced, plural }
 * @param {boolean} props.isVisible - Whether this component is visible (for data fetching)
 */
export default function CustomResourceList({ crdInfo, isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId } = useUI();
    const { handleEditYaml, handleDelete } = useCustomResourceActions(crdInfo);
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

    const { resources, loading, error } = useCustomResources(
        currentContext,
        crdInfo.group,
        crdInfo.version,
        crdInfo.resource,
        selectedNamespaces,
        isVisible,
        crdInfo.namespaced
    );

    // Show namespace column when viewing all namespaces or multiple namespaces (not exactly 1)
    const showNamespaceColumn = crdInfo.namespaced && (selectedNamespaces?.length !== 1);

    const columns = useMemo(() => {
        const cols = [
            {
                key: 'name',
                label: 'Name',
                render: (item) => item.metadata?.name || '-',
                getValue: (item) => item.metadata?.name || ''
            }
        ];

        // Add namespace column when viewing multiple/all namespaces
        if (showNamespaceColumn) {
            cols.push({
                key: 'namespace',
                label: 'Namespace',
                render: (item) => item.metadata?.namespace || '-',
                getValue: (item) => item.metadata?.namespace || ''
            });
        }

        // Add age column
        cols.push({
            key: 'age',
            label: 'Age',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp
        });

        // Add actions column
        cols.push({
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <CustomResourceActionsMenu
                    resource={item}
                    isOpen={activeMenuId === `cr-${item.metadata?.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `cr-${item.metadata?.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        });

        return cols;
    }, [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleDelete, showNamespaceColumn]);

    return (
        <ResourceList
            title={crdInfo.kind}
            columns={columns}
            data={resources}
            isLoading={loading}
            namespaces={crdInfo.namespaced ? namespaces : []}
            currentNamespace={crdInfo.namespaced ? selectedNamespaces : []}
            onNamespaceChange={crdInfo.namespaced ? setSelectedNamespaces : undefined}
            showNamespaceSelector={crdInfo.namespaced}
            multiSelectNamespaces={true}
            initialSort={{ key: 'age', direction: 'desc' }}
            resourceType={`cr-${crdInfo.group}-${crdInfo.resource}`}
        />
    );
}
