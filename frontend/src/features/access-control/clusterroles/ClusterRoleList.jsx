import React, { useMemo, useState, useCallback } from 'react';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import { useClusterRoles } from '../../../hooks/resources';
import { useK8s } from '../../../context/K8sContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteClusterRole, GetClusterRoleYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ClusterRoleActionsMenu from './ClusterRoleActionsMenu';
import { useClusterRoleActions } from './useClusterRoleActions';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function ClusterRoleList({ isVisible }) {
    const { currentContext } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { clusterRoles, loading } = useClusterRoles(currentContext, isVisible);
    const { handleEditYaml } = useClusterRoleActions();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'ClusterRole',
        resourceType: 'clusterroles',
        isNamespaced: false,
        deleteApi: (context, name) => DeleteClusterRole(name),
        getYamlApi: GetClusterRoleYaml,
        currentContext,
    });

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
        {
            key: 'rules',
            label: 'Rules',
            align: 'center',
            render: (item) => (item.rules || []).length,
            getValue: (item) => (item.rules || []).length
        },
        {
            key: 'aggregation',
            label: 'Aggregation',
            render: (item) => {
                const selectors = item.aggregationRule?.clusterRoleSelectors || [];
                return selectors.length > 0 ? (
                    <span className="text-blue-400">Aggregated</span>
                ) : (
                    <span className="text-gray-500">-</span>
                );
            },
            getValue: (item) => (item.aggregationRule?.clusterRoleSelectors || []).length > 0 ? 'Yes' : 'No'
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <ClusterRoleActionsMenu
                    clusterRole={item}
                    isOpen={activeMenuId === `clusterrole-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `clusterrole-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={handleEditYaml}
                    onDelete={(clusterRole) => openBulkDelete([clusterRole])}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, openBulkDelete]);

    return (
        <>
            <ResourceList
                title="Cluster Roles"
                columns={columns}
                data={clusterRoles}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="clusterroles"
                onRowClick={handleEditYaml}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
            />
        </>
    );
}
