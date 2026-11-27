import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import { useValueStore, useSyncValueStore } from '~/utils/valueStore';
import MenuBound from '~/components/shared/MenuBound';
import BulkActionModal from '~/components/shared/BulkActionModal';
import NamespaceActionsMenu from './NamespaceActionsMenu';
import { useNamespacesList } from '~/hooks/resources';
import { useNamespaceActions } from './useNamespaceActions';
import { useNamespaceMetrics } from '~/hooks/useNamespaceMetrics';
import { useK8s } from '~/context';
import { useMenu } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeleteNamespace, GetNamespaceYAML } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { getNamespaceStatus, createNamespaceSemantics } from './semantics';
import { NamespaceMetricCell, NamespaceMetricViewCell, type NamespaceMetricsSnapshot } from './NamespaceMetricCells';
import { useMetricsOverlayConfig, useOverlayAvailability } from '~/hooks/useViewOverlay';
import { OVERLAY_NAMESPACE_METRICS } from '~/lib/views/protocol';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { buildNamespaceViewColumns, getStatusColor, NAMESPACE_HIDDEN_BY_DEFAULT, NAMESPACE_PINNED_COLUMNS } from './viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';

const NAMESPACE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const NAMESPACE_VIEW_OPTIONS = { sort: NAMESPACE_INITIAL_SORT, columns: defaultViewColumns('namespaces', NAMESPACE_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function NamespaceList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const selection = useSelection();

    // Unified bulk actions (also used for single delete)
    const {
        bulkModalProps,
        openBulkDelete,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Namespace',
        resourceType: 'namespaces',
        isNamespaced: false,
        deleteApi: DeleteNamespace,
        getYamlApi: GetNamespaceYAML,

    });
    const { namespaces, loading, loadState, view, viewSnapshot } = useNamespacesList(currentContext, isVisible, NAMESPACE_VIEW_OPTIONS) as any;
    const { handleShowDetails, handleEditYaml } = useStableCallbacks(useNamespaceActions());
    const withObject = useWithFullObject(view ?? null);
    // Details need the namespace object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    // Legacy: load metrics after namespaces are loaded. In view mode the backend polls the
    // metrics source while the view is open and delivers them as the cpu, memory and metrics cells.
    const { metrics, available: metricsAvailable } = useNamespaceMetrics(isVisible && !view, !loading && (view ? false : (namespaces as any)?.length > 0));
    useMetricsOverlayConfig(isVisible && !!view);
    const metricsAvailability = useOverlayAvailability(viewSnapshot, OVERLAY_NAMESPACE_METRICS);

    // Metrics reach cells through a store so ticks never rebuild the column definitions.
    const metricsSnapshot = useMemo<NamespaceMetricsSnapshot>(() => ({ metrics, available: metricsAvailable }), [metrics, metricsAvailable]);
    const metricsStore = useValueStore<NamespaceMetricsSnapshot>(metricsSnapshot);
    useSyncValueStore(metricsStore, metricsSnapshot);
    const namespaceSemantics = useMemo(() => createNamespaceSemantics((item: any) => metricsStore.get().metrics[item.metadata?.name]), [metricsStore]);

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item: any) => item.metadata?.name,
            getValue: namespaceSemantics.name.getValue
        },
        {
            key: 'cpu',
            label: 'CPU',
            volatile: true,
            render: (item: any) => <NamespaceMetricCell store={metricsStore} item={item} kind="cpu" />,
            getValue: namespaceSemantics.cpu.getValue
        },
        {
            key: 'memory',
            label: 'Memory',
            volatile: true,
            render: (item: any) => <NamespaceMetricCell store={metricsStore} item={item} kind="memory" />,
            getValue: namespaceSemantics.memory.getValue
        },
        {
            key: 'status',
            label: 'Status',
            render: (item: any) => {
                const status = getNamespaceStatus(item);
                return <span className={getStatusColor(status)}>{status}</span>;
            },
            getValue: namespaceSemantics.status.getValue
        },
        {
            key: 'age',
            label: 'Age',
            render: (item: any) => formatAge(item.metadata?.creationTimestamp),
            getValue: namespaceSemantics.age.getValue
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={NamespaceActionsMenu}
                    menuId={`namespace-${item.metadata.uid}`}
                    namespace={item}
                    onEditYaml={() => handleEditYaml(item)}
                    onDelete={() => openBulkDelete([item])}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleEditYaml, openBulkDelete, metricsStore, namespaceSemantics]);

    const viewColumns = useMemo(
        () => buildNamespaceViewColumns({
            renderMetric: (item, kind) => <NamespaceMetricViewCell availability={metricsAvailability} item={item} kind={kind} />,
            onEditYaml: handleEditYaml,
            onDelete: item => openBulkDelete([item]),
        }),
        [handleEditYaml, openBulkDelete, metricsAvailability]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Namespaces"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={NAMESPACE_PINNED_COLUMNS}
                data={view ? NO_DATA : namespaces}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={NAMESPACE_INITIAL_SORT}
                resourceType="namespaces"
                volatileInputs={metrics}
                onRowClick={view ? showDetails : handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal
                {...bulkModalProps}
                action="delete"
                actionLabel="Delete"
                onExportYaml={exportYaml}
            />
        </>
    );
}
