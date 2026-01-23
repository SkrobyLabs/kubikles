import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import DaemonSetActionsMenu from './DaemonSetActionsMenu';
import { useDaemonSets } from '../../../hooks/resources';
import { useDaemonSetActions } from './useDaemonSetActions';
import { useK8s } from '../../../context/K8sContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeleteDaemonSet, RestartDaemonSet, GetDaemonSetYaml } from '../../../../wailsjs/go/main/App';
import { formatAge } from '../../../utils/formatting';
import { useMenuPosition } from '../../../hooks/useMenuPosition';

export default function DaemonSetList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        openBulkRestart,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'DaemonSet',
        resourceType: 'daemonsets',
        isNamespaced: true,
        deleteApi: DeleteDaemonSet,
        restartApi: RestartDaemonSet,
        getYamlApi: GetDaemonSetYaml,
        currentContext,
    });
    const { daemonSets, loading } = useDaemonSets(currentContext, selectedNamespaces, isVisible);
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useDaemonSetActions();

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'desired',
            label: 'Desired',
            render: (item) => item.status?.desiredNumberScheduled || 0,
            getValue: (item) => item.status?.desiredNumberScheduled || 0
        },
        {
            key: 'current',
            label: 'Current',
            render: (item) => item.status?.currentNumberScheduled || 0,
            getValue: (item) => item.status?.currentNumberScheduled || 0
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item) => item.status?.numberReady || 0,
            getValue: (item) => item.status?.numberReady || 0
        },
        {
            key: 'available',
            label: 'Available',
            render: (item) => item.status?.numberAvailable || 0,
            getValue: (item) => item.status?.numberAvailable || 0
        },
        { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <DaemonSetActionsMenu
                    daemonSet={item}
                    isOpen={activeMenuId === `ds-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `ds-${item.metadata.uid}`, buttonElement)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onRestart={() => openBulkRestart([item])}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs]);

    return (
        <>
            <ResourceList
                title="DaemonSets"
                columns={columns}
                data={daemonSets}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="daemonsets"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
                onBulkRestart={openBulkRestart}
            />
            <BulkActionModal
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action={bulkActionModal.action}
                actionLabel={bulkActionModal.action === 'delete' ? 'Delete' : 'Restart'}
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={bulkActionModal.action === 'delete' ? exportYaml : null}
                progress={bulkProgress}
            />
        </>
    );
}
