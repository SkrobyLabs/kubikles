import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import DaemonSetActionsMenu from './DaemonSetActionsMenu';
import { useDaemonSets } from '../../../hooks/resources';
import { useDaemonSetActions } from './useDaemonSetActions';
import { useK8s } from '../../../context';
import { useMenu } from '../../../context';
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
        // Hidden by default columns
        {
            key: 'upToDate',
            label: 'Up-to-date',
            defaultHidden: true,
            render: (item) => item.status?.updatedNumberScheduled || 0,
            getValue: (item) => item.status?.updatedNumberScheduled || 0,
        },
        {
            key: 'updateStrategy',
            label: 'Update Strategy',
            defaultHidden: true,
            render: (item) => item.spec?.updateStrategy?.type || 'RollingUpdate',
            getValue: (item) => item.spec?.updateStrategy?.type || 'RollingUpdate',
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (item) => {
                const containers = item.spec?.template?.spec?.containers || [];
                if (containers.length === 0) return '-';
                if (containers.length === 1) return <span title={containers[0].image}>{containers[0].image?.split('/').pop()}</span>;
                return <span title={containers.map(c => c.image).join('\n')}>{containers.length} images</span>;
            },
            getValue: (item) => item.spec?.template?.spec?.containers?.[0]?.image || '',
        },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: (item) => {
                const labels = item.spec?.selector?.matchLabels || {};
                const entries = Object.entries(labels);
                if (entries.length === 0) return '-';
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: (item) => Object.entries(item.spec?.selector?.matchLabels || {}).map(([k, v]) => `${k}=${v}`).join(','),
        },
        {
            key: 'nodeSelector',
            label: 'Node Selector',
            defaultHidden: true,
            render: (item) => {
                const selector = item.spec?.template?.spec?.nodeSelector || {};
                const entries = Object.entries(selector);
                if (entries.length === 0) return <span className="text-gray-500">-</span>;
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: (item) => Object.entries(item.spec?.template?.spec?.nodeSelector || {}).map(([k, v]) => `${k}=${v}`).join(','),
        },
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
