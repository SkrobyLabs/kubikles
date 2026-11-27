import React, { useMemo, useState, useCallback, useEffect } from 'react';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import { useNodes } from '~/hooks/resources';
import { useNodeMetrics } from '~/hooks/useNodeMetrics';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { useK8s, useNotification } from '~/context';
import { formatAge } from '~/utils/formatting';
import { EllipsisVerticalIcon, TableCellsIcon, Squares2X2Icon, NoSymbolIcon, CheckCircleIcon } from '@heroicons/react/24/outline';
import { SetNodeSchedulable, DeleteNode, GetNodeYaml } from 'wailsjs/go/main/App';
import NodeActionsMenu from './NodeActionsMenu';
import { hasCordonTargets } from './bulkCordon';
import { useNodeActions } from './useNodeActions';
import MenuBound from '~/components/shared/MenuBound';
import { NodeMetricCell, NodeMetricViewCell, type NodeMetricsSnapshot } from './NodeMetricCells';
import { ConditionsCell, TaintsCell } from './NodeCells';
import { createNodeSemantics } from './semantics';
import { buildNodeViewColumns, NODE_HIDDEN_BY_DEFAULT, NODE_PINNED_COLUMNS } from './viewColumns';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { useAllViewObjects, useWithFullObject } from '~/hooks/useViewObject';
import { useMetricsOverlayConfig, useOverlayAvailability } from '~/hooks/useViewOverlay';
import { OVERLAY_NODE_METRICS } from '~/lib/views/protocol';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import { useValueStore, useSyncValueStore } from '~/utils/valueStore';
import NodeTopology from '../topology/NodeTopology';

const STORAGE_KEY = 'kubikles-nodes-view';

const NODE_INITIAL_SORT = { key: 'age', direction: 'desc' };
const NODE_VIEW_OPTIONS = { sort: NODE_INITIAL_SORT, columns: defaultViewColumns('nodes', NODE_HIDDEN_BY_DEFAULT) };
const NO_DATA: any[] = [];

export default function NodeList({ isVisible }: { isVisible: boolean }) {
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();
    const { nodes: listNodes, loading, refetch, loadState, view, viewSnapshot } = useNodes(currentContext, isVisible, NODE_VIEW_OPTIONS) as any;

    // View mode: list or topology, persisted to localStorage
    const [viewMode, setViewMode] = useState<'list' | 'topology'>(() =>
        (localStorage.getItem(STORAGE_KEY) as 'list' | 'topology') || 'list'
    );

    useEffect(() => {
        localStorage.setItem(STORAGE_KEY, viewMode);
    }, [viewMode]);

    // The topology diagram draws from the whole node list and the metrics map. With the Go view on,
    // the node objects come from the view (refs stream plus the object route over the shared store
    // shard), so no legacy node list or watch runs; the metrics map stays on the metrics hook, which
    // is a poll of the metrics source and not a node list. Without the view the list hook feeds both.
    const topologyOnView = !!view && viewMode === 'topology';
    const viewNodes = useAllViewObjects(view ?? null, viewSnapshot ?? null, isVisible && topologyOnView);
    // The topology draws every node: the view's search and column filters were set by the list
    // ResourceList, which is unmounted in topology mode, so clear them (the list pushes its own
    // values again when it mounts).
    useEffect(() => {
        if (topologyOnView && view) view.update({ search: '', columnFilters: [] });
    }, [topologyOnView, view]);
    const nodes: any[] = view ? (topologyOnView ? viewNodes.objects : NO_DATA) : listNodes;
    const nodesLoading: boolean = topologyOnView ? viewNodes.loading : loading;

    // Delay metrics fetch until nodes are loaded to prioritize showing node list first
    const { metrics, available: metricsAvailable } = useNodeMetrics(isVisible && (!view || topologyOnView), !nodesLoading && nodes.length > 0);
    // In view mode the backend polls the metrics source while the view is open.
    useMetricsOverlayConfig(isVisible && !!view);
    const metricsAvailability = useOverlayAvailability(viewSnapshot, OVERLAY_NODE_METRICS);
    // Metrics reach cells through a store so ticks never rebuild the column definitions.
    const metricsSnapshot = useMemo<NodeMetricsSnapshot>(() => ({ metrics, available: metricsAvailable }), [metrics, metricsAvailable]);
    const metricsStore = useValueStore<NodeMetricsSnapshot>(metricsSnapshot);
    useSyncValueStore(metricsStore, metricsSnapshot);
    const semantics = useMemo(() => createNodeSemantics((item: any) => metricsStore.get().metrics[item.metadata?.name]), [metricsStore]);

    const nodeActions = useNodeActions(refetch);
    const { handleShowDetails, handleEditYaml, handleCordonUncordon, handleShell, handleDelete } = nodeActions;
    const stable = useStableCallbacks({ handleEditYaml, handleCordonUncordon, handleShell, handleDelete });
    const withObject = useWithFullObject(view ?? null);
    // Details need the node object itself; windowed rows only carry identity and cells.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    // Multi-select
    const selection = useSelection();
    const cordonNode = useCallback((name: string) => SetNodeSchedulable(name, false), []);
    const uncordonNode = useCallback((name: string) => SetNodeSchedulable(name, true), []);
    const {
        bulkModalProps,
        bulkActionModal,
        openBulkCustomAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Node',
        resourceType: 'nodes',
        isNamespaced: false,
        deleteApi: DeleteNode,
        customApis: { cordon: cordonNode, uncordon: uncordonNode },
        getYamlApi: GetNodeYaml,
    });

    // Compute cordon/uncordon button visibility from selected items. A windowed list holds only
    // the identity of selected rows, so both buttons show for any selection and the click picks
    // the nodes the action applies to from their objects.
    const windowed = !!view && viewMode === 'list';
    const selectedItems = windowed ? [] : selection.getSelectedItems(nodes);
    const hasSelection = windowed ? selection.selectedIds.size > 0 : selectedItems.length > 0;
    const hasSchedulable = windowed || selectedItems.some((n: any) => !n.spec?.unschedulable);
    const hasCordoned = windowed || selectedItems.some((n: any) => n.spec?.unschedulable === true);

    const runBulkCordon = useCallback((action: 'cordon' | 'uncordon') => {
        if (!windowed) {
            openBulkCustomAction(selectedItems, action);
            return;
        }
        const refs = selection.getRefItems ? selection.getRefItems() : [];
        view.getObjects(refs).then((objects: any[]) => {
            // Like the legacy list, the modal gets every selected node. The legacy list hides a
            // button when no selected node qualifies; here the buttons always show, so say why
            // nothing opens instead.
            if (!hasCordonTargets(objects, action)) {
                addNotification({ type: 'info', title: action === 'cordon' ? 'Nothing to cordon' : 'Nothing to uncordon', message: action === 'cordon' ? 'All selected nodes are already cordoned' : 'None of the selected nodes are cordoned' });
                return;
            }
            openBulkCustomAction(objects, action);
        }).catch((err: unknown) => {
            addNotification({ type: 'error', title: 'Failed to load nodes', message: String((err as Error)?.message ?? err) });
        });
    }, [windowed, selectedItems, selection, view, openBulkCustomAction, addNotification]);

    const bulkCustomActions = hasSelection ? (
        <>
            {hasSchedulable && (
                <button
                    onClick={() => runBulkCordon('cordon')}
                    className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-yellow-400 hover:text-yellow-300 hover:bg-yellow-500/10 rounded transition-colors"
                    title="Cordon selected nodes"
                >
                    <NoSymbolIcon className="h-4 w-4" />
                    <span>Cordon</span>
                </button>
            )}
            {hasCordoned && (
                <button
                    onClick={() => runBulkCordon('uncordon')}
                    className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-green-400 hover:text-green-300 hover:bg-green-500/10 rounded transition-colors"
                    title="Uncordon selected nodes"
                >
                    <CheckCircleIcon className="h-4 w-4" />
                    <span>Uncordon</span>
                </button>
            )}
        </>
    ) : null;

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: semantics.name.getValue },
        {
            key: 'cpu',
            label: 'CPU',
            filterType: 'numeric',
            numericHint: 'CPU usage % (0-100)',
            numericUnit: '%',
            volatile: true,
            render: (item: any) => <NodeMetricCell store={metricsStore} item={item} kind="cpu" />,
            getValue: semantics.cpu.getValue,
            getNumericValue: semantics.cpu.getNumericValue
        },
        {
            key: 'memory',
            label: 'Memory',
            filterType: 'numeric',
            numericHint: 'Memory usage % (0-100)',
            numericUnit: '%',
            volatile: true,
            render: (item: any) => <NodeMetricCell store={metricsStore} item={item} kind="memory" />,
            getValue: semantics.memory.getValue,
            getNumericValue: semantics.memory.getNumericValue
        },
        {
            key: 'pods',
            label: 'Pods',
            volatile: true,
            render: (item: any) => <NodeMetricCell store={metricsStore} item={item} kind="pods" />,
            getValue: semantics.pods.getValue
        },
        {
            key: 'conditions',
            label: 'Conditions',
            render: (item: any) => <ConditionsCell node={item} />,
            getValue: semantics.conditions.getValue
        },
        {
            key: 'taints',
            label: 'Taints',
            align: 'center',
            filterable: false,
            render: (item: any) => <TaintsCell node={item} />,
            getValue: semantics.taints.getValue
        },
        { key: 'version', label: 'Version', render: (item: any) => item.status?.nodeInfo?.kubeletVersion, getValue: semantics.version.getValue },
        // Hidden by default columns
        {
            key: 'roles',
            label: 'Roles',
            defaultHidden: true,
            render: (item: any) => {
                const labels = item.metadata?.labels || {};
                const roles = Object.keys(labels)
                    .filter((k: any) => k.startsWith('node-role.kubernetes.io/'))
                    .map((k: any) => k.replace('node-role.kubernetes.io/', ''));
                return roles.length > 0 ? roles.join(', ') : <span className="text-gray-500">-</span>;
            },
            getValue: semantics.roles.getValue,
        },
        {
            key: 'internalIP',
            label: 'Internal IP',
            defaultHidden: true,
            render: (item: any) => {
                const addr = (item.status?.addresses || []).find((a: any) => a.type === 'InternalIP');
                return addr?.address || <span className="text-gray-500">-</span>;
            },
            getValue: semantics.internalIP.getValue,
        },
        {
            key: 'externalIP',
            label: 'External IP',
            defaultHidden: true,
            render: (item: any) => {
                const addr = (item.status?.addresses || []).find((a: any) => a.type === 'ExternalIP');
                return addr?.address || <span className="text-gray-500">-</span>;
            },
            getValue: semantics.externalIP.getValue,
        },
        {
            key: 'osImage',
            label: 'OS Image',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.osImage || '-',
            getValue: semantics.osImage.getValue,
        },
        {
            key: 'kernelVersion',
            label: 'Kernel',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.kernelVersion || '-',
            getValue: semantics.kernelVersion.getValue,
        },
        {
            key: 'containerRuntime',
            label: 'Container Runtime',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.containerRuntimeVersion || '-',
            getValue: semantics.containerRuntime.getValue,
        },
        {
            key: 'architecture',
            label: 'Arch',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.architecture || '-',
            getValue: semantics.architecture.getValue,
        },
        {
            key: 'os',
            label: 'OS',
            defaultHidden: true,
            render: (item: any) => item.status?.nodeInfo?.operatingSystem || '-',
            getValue: semantics.os.getValue,
        },
        {
            key: 'age',
            label: 'Age',
            filterType: 'numeric',
            numericHint: 'Age in hours',
            numericUnit: 'h',
            render: (item: any) => formatAge(item.metadata?.creationTimestamp),
            getValue: semantics.age.getValue,
            getNumericValue: semantics.age.getNumericValue
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={NodeActionsMenu}
                    menuId={`node-${item.metadata.uid}`}
                    node={item}
                    onEditYaml={handleEditYaml}
                    onCordonUncordon={handleCordonUncordon}
                    onShell={handleShell}
                    onDelete={handleDelete}
                />
            ),
            getValue: () => '',
            isColumnSelector: true,
            disableSort: true
        }
    ] as any[], [metricsStore, semantics, handleEditYaml, handleCordonUncordon, handleShell, handleDelete]);

    const viewColumns = useMemo(
        () => buildNodeViewColumns({
            renderMetric: (item, kind) => <NodeMetricViewCell availability={metricsAvailability} item={item} kind={kind} />,
            onEditYaml: (item: any) => stable.handleEditYaml(item),
            onCordonUncordon: (item: any) => stable.handleCordonUncordon(item),
            onShell: (item: any) => stable.handleShell(item),
            onDelete: (item: any) => stable.handleDelete(item),
        }),
        [metricsAvailability, stable]);

    const viewToggle = (
        <div className="flex items-center gap-0.5 bg-surface rounded-md p-0.5 border border-border">
            <button
                onClick={() => setViewMode('list')}
                className={`p-1.5 rounded transition-colors ${viewMode === 'list' ? 'bg-surface-light text-white' : 'text-gray-500 hover:text-gray-300'}`}
                title="List view"
            >
                <TableCellsIcon className="h-4 w-4" />
            </button>
            <button
                onClick={() => setViewMode('topology')}
                className={`p-1.5 rounded transition-colors ${viewMode === 'topology' ? 'bg-surface-light text-white' : 'text-gray-500 hover:text-gray-300'}`}
                title="Topology view"
            >
                <Squares2X2Icon className="h-4 w-4" />
            </button>
        </div>
    );

    if (viewMode === 'topology') {
        return (
            <ResourceList loadState={loadState}
                title="Nodes"
                columns={columns}
                data={nodes}
                isLoading={false}
                showNamespaceSelector={false}
                initialSort={NODE_INITIAL_SORT}
                resourceType="nodes"
                onRowClick={view ? showDetails : handleShowDetails}
                volatileInputs={metrics}
                customHeaderActions={viewToggle}
                customBody={
                    <NodeTopology
                        isVisible={isVisible}
                        nodes={nodes}
                        nodesLoading={nodesLoading}
                        metrics={metrics}
                        metricsAvailable={metricsAvailable}
                        nodeActions={nodeActions}
                    />
                }
            />
        );
    }

    return (
        <>
            <ResourceList
                title="Nodes"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={NODE_PINNED_COLUMNS}
                data={view ? NO_DATA : nodes}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={NODE_INITIAL_SORT}
                resourceType="nodes"
                onRowClick={view ? showDetails : handleShowDetails}
                volatileInputs={metrics}
                customHeaderActions={viewToggle}
                selectable={true}
                selection={selection}
                onBulkExportYaml={exportYaml}
                bulkCustomActions={bulkCustomActions}
            />
            <BulkActionModal
                {...bulkModalProps}
                action={bulkActionModal.action || 'cordon'}
                actionLabel={bulkActionModal.action === 'uncordon' ? 'Uncordon' : 'Cordon'}
                onExportYaml={exportYaml}
            />
        </>
    );
}
