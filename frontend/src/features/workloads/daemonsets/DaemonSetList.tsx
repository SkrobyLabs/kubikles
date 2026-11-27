import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import DaemonSetActionsMenu from './DaemonSetActionsMenu';
import { useDaemonSets } from '~/hooks/resources';
import { useDaemonSetActions } from './useDaemonSetActions';
import { useK8s } from '~/context';
import { useMenu } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteDaemonSet, RestartDaemonSet, GetDaemonSetYaml } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { daemonSetSemantics } from './semantics';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildDaemonSetViewColumns, DAEMONSET_HIDDEN_BY_DEFAULT, DAEMONSET_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const DAEMONSET_INITIAL_SORT = { key: 'age', direction: 'desc' };
const DAEMONSET_VIEW_OPTIONS = { sort: DAEMONSET_INITIAL_SORT, columns: defaultViewColumns('daemonsets', DAEMONSET_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function DaemonSetList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkModalProps,
        openBulkDelete,
        openBulkRestart,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'DaemonSet',
        resourceType: 'daemonsets',
        isNamespaced: true,
        deleteApi: DeleteDaemonSet,
        restartApi: RestartDaemonSet,
        getYamlApi: GetDaemonSetYaml,

    });
    const { daemonSets, loading, loadState, view } = useDaemonSets(currentContext, selectedNamespaces, isVisible, DAEMONSET_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml, handleShowDependencies, handleViewLogs } = useStableCallbacks(useDaemonSetActions());

    const withObject = useWithFullObject(view ?? null);
    // Details need the object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: daemonSetSemantics.name.getValue, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: daemonSetSemantics.namespace.getValue },
        {
            key: 'desired',
            label: 'Desired',
            render: (item: any) => item.status?.desiredNumberScheduled || 0,
            getValue: daemonSetSemantics.desired.getValue
        },
        {
            key: 'current',
            label: 'Current',
            render: (item: any) => item.status?.currentNumberScheduled || 0,
            getValue: daemonSetSemantics.current.getValue
        },
        {
            key: 'ready',
            label: 'Ready',
            render: (item: any) => item.status?.numberReady || 0,
            getValue: daemonSetSemantics.ready.getValue
        },
        {
            key: 'available',
            label: 'Available',
            render: (item: any) => item.status?.numberAvailable || 0,
            getValue: daemonSetSemantics.available.getValue
        },
        { key: 'age', label: 'Age', render: (item: any) => formatAge(item.metadata?.creationTimestamp), getValue: daemonSetSemantics.age.getValue },
        // Hidden by default columns
        {
            key: 'upToDate',
            label: 'Up-to-date',
            defaultHidden: true,
            render: (item: any) => item.status?.updatedNumberScheduled || 0,
            getValue: daemonSetSemantics.upToDate.getValue,
        },
        {
            key: 'updateStrategy',
            label: 'Update Strategy',
            defaultHidden: true,
            render: (item: any) => item.spec?.updateStrategy?.type || 'RollingUpdate',
            getValue: daemonSetSemantics.updateStrategy.getValue,
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (item: any) => {
                const containers = item.spec?.template?.spec?.containers || [];
                if (containers.length === 0) return '-';
                if (containers.length === 1) return <span title={containers[0].image}>{containers[0].image?.split('/').pop()}</span>;
                return <span title={containers.map((c: any) => c.image).join('\n')}>{containers.length} images</span>;
            },
            getValue: daemonSetSemantics.image.getValue,
        },
        {
            key: 'selector',
            label: 'Selector',
            defaultHidden: true,
            render: (item: any) => {
                const labels = item.spec?.selector?.matchLabels || {};
                const entries = Object.entries(labels);
                if (entries.length === 0) return '-';
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: daemonSetSemantics.selector.getValue,
        },
        {
            key: 'nodeSelector',
            label: 'Node Selector',
            defaultHidden: true,
            render: (item: any) => {
                const selector = item.spec?.template?.spec?.nodeSelector || {};
                const entries = Object.entries(selector);
                if (entries.length === 0) return <span className="text-gray-500">-</span>;
                return <span title={entries.map(([k, v]) => `${k}=${v}`).join('\n')}>{entries.length} label{entries.length > 1 ? 's' : ''}</span>;
            },
            getValue: daemonSetSemantics.nodeSelector.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={DaemonSetActionsMenu}
                    menuId={`ds-${item.metadata.uid}`}
                    daemonSet={item}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onRestart={() => openBulkRestart?.([ item])}
                    onDelete={() => openBulkDelete([item])}
                    onViewLogs={() => handleViewLogs(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs]);

    const viewColumns = useMemo(
        () => buildDaemonSetViewColumns({
            onEditYaml: handleEditYaml as (item: any) => void,
            onShowDependencies: handleShowDependencies as (item: any) => void,
            onRestart: item => openBulkRestart?.([item]),
            onDelete: item => openBulkDelete([item]),
            onViewLogs: handleViewLogs as (item: any) => void,
        }),
        [handleEditYaml, handleShowDependencies, openBulkRestart, openBulkDelete, handleViewLogs]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="DaemonSets"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={DAEMONSET_PINNED_COLUMNS}
                data={view ? NO_DATA : daemonSets}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                initialSort={DAEMONSET_INITIAL_SORT}
                resourceType="daemonsets"
                onRowClick={view ? showDetails : handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
                onBulkRestart={openBulkRestart}
            />
            <BulkActionModal
                {...bulkModalProps}
                action={bulkActionModal.action || ''}
                actionLabel={bulkActionModal.action === 'delete' ? 'Delete' : 'Restart'}
                onExportYaml={bulkActionModal.action === 'delete' ? exportYaml : undefined}
            />
        </>
    );
}
