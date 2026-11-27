import React, { useMemo, useState, useCallback, useEffect, useRef } from 'react';
import { EllipsisVerticalIcon, BellAlertIcon, BellSlashIcon, ArrowRightStartOnRectangleIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import MenuBound from '~/components/shared/MenuBound';
import { useStableCallbacks } from '~/hooks/useStableCallbacks';
import { useValueStore, useSyncValueStore, type ValueStore } from '~/utils/valueStore';
import { PodMetricCell, PodMetricViewCell, type PodMetricsSnapshot } from './PodMetricCells';
import { ContainerSquares, PodControllerLink, PodStatusText, renderImages } from './PodCells';
import { createPodSemantics } from './semantics';
import { buildPodViewColumns, POD_HIDDEN_BY_DEFAULT, POD_PINNED_COLUMNS } from './viewColumns';
import { defaultViewColumns } from '~/lib/views/viewColumns';
import { useWithFullObject } from '~/hooks/useViewObject';
import { useMetricsOverlayConfig, useOverlayAvailability } from '~/hooks/useViewOverlay';
import { OVERLAY_POD_METRICS } from '~/lib/views/protocol';
import BulkActionModal from '~/components/shared/BulkActionModal';
import PodPortForwardDialog from '~/components/shared/PodPortForwardDialog';
import NotificationSettingsMenu from '~/components/shared/NotificationSettingsMenu';
import PodActionsMenu from './PodActionsMenu';
import { usePods } from '~/hooks/resources';
import { usePodMetrics } from '~/hooks/usePodMetrics';
import { usePodActions } from './usePodActions';
import { evictablePods } from './evictable';
import { useK8s } from '~/context';
import { useUI, useNotification } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { DeletePod, EvictPod, GetPodYaml, ListPods } from 'wailsjs/go/main/App';
import { formatAge } from '~/utils/formatting';
import { getPodStatus, getPodController } from '~/utils/k8s-helpers';
import { usePodNotifications, usePodViewNotifications, warmUpAudio, playNotificationSound } from '~/hooks/usePodNotifications';
import { ALL_PODS } from '~/components/shared/log-viewer';
import { getLogContainerNames } from '~/components/shared/log-viewer/logTarget';

// Owner link cell. CRDs arrive asynchronously, so it reads them from a store: when they load
// only these cells re-render, the columns are not rebuilt.
const PodControllerCell = React.memo(function PodControllerCell({ item, crdsStore, navigateWithSearch }: { item: any; crdsStore: ValueStore<any>; navigateWithSearch: (viewId: string, search: string) => void }) {
    return <PodControllerLink controller={getPodController(item)} crdsStore={crdsStore} navigateWithSearch={navigateWithSearch} />;
});

const POD_INITIAL_SORT = { key: 'age', direction: 'desc' };
const POD_VIEW_COLUMNS = defaultViewColumns('pods', POD_HIDDEN_BY_DEFAULT);
const POD_VIEW_OPTIONS_BASE = { sort: POD_INITIAL_SORT, columns: POD_VIEW_COLUMNS };
const POD_VIEW_OPTIONS_NOTIFY = { ...POD_VIEW_OPTIONS_BASE, notifyOn: ['pods'] };
const NO_DATA: any[] = [];

export default function PodList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();
    const { addNotification } = useNotification();

    // Load CRDs for owner reference resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkModalProps,
        openBulkDelete,
        openBulkCustomAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Pod',
        resourceType: 'pods',
        isNamespaced: true,
        deleteApi: DeletePod,
        customApis: {
            evict: EvictPod,
        },
        getYamlApi: GetPodYaml,

    });
    // Pod notifications toggle and settings (persisted to localStorage)
    const [notificationsEnabled, setNotificationsEnabled] = useState(() => {
        try { return localStorage.getItem('kubikles_pod_notifications') === 'true'; }
        catch { return false; }
    });
    const [notificationSound, setNotificationSound] = useState(() => {
        try { return localStorage.getItem('kubikles_pod_notification_sound') || 'alert'; }
        catch { return 'alert'; }
    });
    const [notificationThrottle, setNotificationThrottle] = useState(() => {
        try { return parseInt(localStorage.getItem('kubikles_pod_notification_throttle') || '0', 10); }
        catch { return 0; }
    });
    const { pods: rawPods, loading, loadState, view, viewSnapshot } = usePods(currentContext, selectedNamespaces, isVisible, notificationsEnabled ? POD_VIEW_OPTIONS_NOTIFY : POD_VIEW_OPTIONS_BASE) as any;
    const pods: any[] = rawPods || [];
    // Delay metrics fetch until pods are loaded to prioritize showing pod list first. In view
    // mode the backend polls the metrics source while the view is open.
    const { metrics, available: metricsAvailable } = usePodMetrics(isVisible && !view, !loading && pods.length > 0);
    useMetricsOverlayConfig(isVisible && !!view);
    const metricsAvailability = useOverlayAvailability(viewSnapshot, OVERLAY_POD_METRICS);
    const { openLogs, handleShell, handleFiles, handleEditYaml, handleShowDependencies, handleShowDetails, handleEvict } = useStableCallbacks(usePodActions());
    const withObject = useWithFullObject(view ?? null);
    // Shell, files, details, logs and port forwarding read spec or status; windowed rows only
    // carry identity and cells, so those handlers receive the full object.
    const showDetails = useMemo(() => withObject(handleShowDetails), [withObject, handleShowDetails]);

    // Column definitions must stay referentially stable (the table re-filters and re-sorts when
    // they change), so everything that changes often reaches cells through refs and stores:
    // metrics ticks, the CRD list (owner links) and the pod list (log sibling lookup).
    const metricsSnapshot = useMemo<PodMetricsSnapshot>(() => ({ metrics, available: metricsAvailable }), [metrics, metricsAvailable]);
    const metricsStore = useValueStore<PodMetricsSnapshot>(metricsSnapshot);
    useSyncValueStore(metricsStore, metricsSnapshot);
    const semantics = useMemo(() => createPodSemantics((item: any) => metricsStore.get().metrics[`${item.metadata?.namespace}/${item.metadata?.name}`]), [metricsStore]);
    const crdsStore = useValueStore<any>(crds);
    useSyncValueStore(crdsStore, crds);
    const podsRef = useRef(pods);
    podsRef.current = pods;
    const { navigateWithSearch: stableNavigate } = useStableCallbacks({ navigateWithSearch });

    const [notificationSettingsMenu, setNotificationSettingsMenu] = useState({ open: false, x: 0, y: 0 });

    useEffect(() => {
        try { localStorage.setItem('kubikles_pod_notifications', String(notificationsEnabled)); }
        catch { /* ignore */ }
    }, [notificationsEnabled]);
    useEffect(() => {
        try { localStorage.setItem('kubikles_pod_notification_sound', notificationSound); }
        catch { /* ignore */ }
    }, [notificationSound]);
    useEffect(() => {
        try { localStorage.setItem('kubikles_pod_notification_throttle', String(notificationThrottle)); }
        catch { /* ignore */ }
    }, [notificationThrottle]);

    const [filteredUids, setFilteredUids] = useState<Set<string> | null>(null);
    const handleFilteredUidsChange = useCallback((uids: any) => setFilteredUids(uids), []);
    // Legacy lists diff the whole pod list (narrowed by the list's filters). A windowed list does
    // not hold it: the Go view finds the restarts, deletions and new pods itself (notifyOn "pods",
    // restarts and deletions of pods the search and filters show) and reports them as transitions.
    usePodNotifications(pods as any, notificationsEnabled && !view, filteredUids, {
        soundKey: notificationSound,
        throttleSeconds: notificationThrottle,
    });
    usePodViewNotifications(view ?? null, notificationsEnabled, {
        soundKey: notificationSound,
        throttleSeconds: notificationThrottle,
    });

    const selectedPods = useMemo(() => selection.getSelectedItems(pods), [selection, pods]);
    const selectedEvictablePods = useMemo(() => evictablePods(selectedPods), [selectedPods]);
    // A windowed list holds only the identity of selected rows: the button shows for any
    // selection and the click picks the evictable pods from their objects.
    const canBulkEvict = view ? selection.selectedIds.size > 0 : selectedEvictablePods.length > 0;

    const bulkEvict = useCallback(() => {
        if (!view) {
            openBulkCustomAction(selectedEvictablePods, 'evict');
            return;
        }
        const refs = selection.getRefItems ? selection.getRefItems() : [];
        view.getObjects(refs).then((objects: any[]) => {
            const evictable = evictablePods(objects);
            if (evictable.length === 0) {
                addNotification({ type: 'info', title: 'Nothing to evict', message: 'None of the selected pods can be evicted' });
                return;
            }
            openBulkCustomAction(evictable, 'evict');
        }).catch((err: unknown) => {
            addNotification({ type: 'error', title: 'Failed to load pods', message: String((err as Error)?.message ?? err) });
        });
    }, [view, selection, selectedEvictablePods, openBulkCustomAction, addNotification]);

    const bulkPodActions = useMemo(() => {
        if (!canBulkEvict) return null;
        return (
            <button
                onClick={bulkEvict}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-orange-400 hover:text-orange-300 hover:bg-orange-500/10 rounded transition-colors"
                title="Evict selected pods that are still running or pending"
            >
                <ArrowRightStartOnRectangleIcon className="h-4 w-4" />
                <span>Evict</span>
            </button>
        );
    }, [canBulkEvict, bulkEvict]);

    const handleBellClick = useCallback((e: any) => {
        if (e.shiftKey) {
            // Shift+click opens settings
            e.preventDefault();
            const rect = e.currentTarget.getBoundingClientRect();
            setNotificationSettingsMenu({ open: true, x: rect.left, y: rect.bottom + 4 });
        } else {
            // Normal click toggles notifications
            warmUpAudio();
            setNotificationsEnabled(prev => !prev);
        }
    }, []);

    const handleBellRightClick = useCallback((e: any) => {
        e.preventDefault();
        setNotificationSettingsMenu({ open: true, x: e.clientX, y: e.clientY });
    }, []);

    const handlePreviewSound = useCallback((soundKey: string) => {
        warmUpAudio();
        playNotificationSound(soundKey);
    }, []);

    // Port forward state
    const [portForwardDialog, setPortForwardDialog] = useState<{ open: boolean; pod: any; port: any }>({ open: false, pod: null, port: null });
    const [portSelectMenu, setPortSelectMenu] = useState<{ open: boolean; pod: any; ports: any[] }>({ open: false, pod: null, ports: [] });

    const handlePortForward = useCallback((pod: any) => {
        const allPorts: any[] = [];
        const allContainers = [
            ...(pod.spec?.initContainers || []),
            ...(pod.spec?.containers || [])
        ];
        for (const container of allContainers) {
            for (const port of container.ports || []) {
                allPorts.push({ ...port, containerName: container.name });
            }
        }
        if (allPorts.length === 0) return;
        if (allPorts.length === 1) {
            setPortForwardDialog({ open: true, pod, port: allPorts[0] });
        } else {
            setPortSelectMenu({ open: true, pod, ports: allPorts });
        }
    }, []);

    // Opens the log viewer with the sibling pods of the pod's controller, read from `siblings`
    // at click time: the column definitions must not depend on the pod list.
    const openPodLogs = useCallback((pod: any, siblings: any[]) => {
        const containers = getLogContainerNames(pod);
        const controller = getPodController(pod);
        let siblingPods: string[] = [];
        const podContainerMap: Record<string, string[]> = {};
        if (controller) {
            const relatedPods = siblings.filter((p: any) => {
                const c = getPodController(p);
                return c && c.uid === controller.uid;
            });
            siblingPods = relatedPods.map((p: any) => p.metadata.name);
            for (const related of relatedPods) {
                podContainerMap[related.metadata.name] = getLogContainerNames(related);
            }
        } else {
            siblingPods = [pod.metadata.name];
        }
        podContainerMap[pod.metadata.name] = containers;

        openLogs(pod.metadata.namespace, pod.metadata.name, containers, siblingPods, podContainerMap, '', pod.metadata.creationTimestamp);
    }, [openLogs]);

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item: any) => item.metadata?.name, getValue: semantics.name.getValue, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item: any) => item.metadata?.namespace, getValue: semantics.namespace.getValue },
        {
            key: 'cpu',
            label: 'CPU',
            filterType: 'numeric',
            numericHint: 'CPU usage % (0-100)',
            numericUnit: '%',
            volatile: true,
            render: (item: any) => <PodMetricCell store={metricsStore} item={item} kind="cpu" />,
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
            render: (item: any) => <PodMetricCell store={metricsStore} item={item} kind="memory" />,
            getValue: semantics.memory.getValue,
            getNumericValue: semantics.memory.getNumericValue
        },
        {
            key: 'containers',
            label: 'Containers',
            filterable: false,
            render: (item: any) => <ContainerSquares statuses={item.status?.containerStatuses || []} />,
            getValue: semantics.containers.getValue
        },
        {
            key: 'status',
            label: 'Status',
            render: (item: any) => <PodStatusText status={getPodStatus(item)} />,
            getValue: semantics.status.getValue,
            getFilterValue: semantics.status.getFilterValue,
        },
        {
            key: 'restarts',
            label: 'Restarts',
            filterType: 'numeric',
            numericHint: 'Total restart count',
            render: (item: any) => semantics.restarts.getValue(item),
            getValue: semantics.restarts.getValue,
            getNumericValue: semantics.restarts.getNumericValue
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
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item: any) => <PodControllerCell item={item} crdsStore={crdsStore} navigateWithSearch={stableNavigate} />,
            getValue: semantics.controlledBy.getValue
        },
        // Hidden by default columns
        {
            key: 'qos',
            label: 'QoS',
            defaultHidden: true,
            render: (item: any) => item.status?.qosClass || '-',
            getValue: semantics.qos.getValue,
        },
        {
            key: 'node',
            label: 'Node',
            defaultHidden: true,
            render: (item: any) => item.spec?.nodeName || <span className="text-gray-500">-</span>,
            getValue: semantics.node.getValue,
        },
        {
            key: 'podIP',
            label: 'Pod IP',
            defaultHidden: true,
            render: (item: any) => item.status?.podIP || <span className="text-gray-500">-</span>,
            getValue: semantics.podIP.getValue,
        },
        {
            key: 'hostIP',
            label: 'Host IP',
            defaultHidden: true,
            render: (item: any) => item.status?.hostIP || <span className="text-gray-500">-</span>,
            getValue: semantics.hostIP.getValue,
        },
        {
            key: 'serviceAccount',
            label: 'Service Account',
            defaultHidden: true,
            render: (item: any) => item.spec?.serviceAccountName || 'default',
            getValue: semantics.serviceAccount.getValue,
        },
        {
            key: 'priorityClass',
            label: 'Priority Class',
            defaultHidden: true,
            render: (item: any) => item.spec?.priorityClassName || <span className="text-gray-500">-</span>,
            getValue: semantics.priorityClass.getValue,
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (item: any) => renderImages((item.spec?.containers || []).map((c: any) => c.image)),
            getValue: semantics.image.getValue,
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <MenuBound
                    as={PodActionsMenu}
                    menuId={`pod-${item.metadata.uid}`}
                    pod={item}
                    onLogs={() => openPodLogs(item, podsRef.current)}
                    onShell={() => handleShell(item)}
                    onFiles={() => handleFiles(item)}
                    onDelete={() => openBulkDelete([item])}
                    onForceDelete={() => openBulkDelete([item])}
                    onEvict={() => handleEvict(item)}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onShowDetails={() => handleShowDetails(item)}
                    onPortForward={() => handlePortForward(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [metricsStore, semantics, crdsStore, stableNavigate, openPodLogs, handleShell, handleFiles, openBulkDelete, handleEvict, handleEditYaml, handleShowDependencies, handleShowDetails, handlePortForward]);

    // View mode: handlers that read spec or status get the full object; the siblings of the
    // logs viewer are listed from the pod's namespace at click time.
    const viewColumns = useMemo(() => {
        const logs = withObject((pod: any) => {
            if (!getPodController(pod)) {
                openPodLogs(pod, []);
                return;
            }
            Promise.resolve(ListPods('', pod.metadata.namespace))
                .then((list: any) => openPodLogs(pod, list || []))
                .catch(() => openPodLogs(pod, []));
        });
        return buildPodViewColumns({
            renderMetric: (item, kind) => <PodMetricViewCell availability={metricsAvailability} item={item} kind={kind} />,
            crdsStore,
            navigateWithSearch: stableNavigate,
            onLogs: logs,
            onShell: withObject(handleShell),
            onFiles: withObject(handleFiles),
            onDelete: item => openBulkDelete([item]),
            onEvict: item => handleEvict(item),
            onEditYaml: item => handleEditYaml(item),
            onShowDependencies: item => handleShowDependencies(item),
            onShowDetails: showDetails,
            onPortForward: withObject(handlePortForward),
        });
    }, [metricsAvailability, crdsStore, stableNavigate, withObject, openPodLogs, handleShell, handleFiles, openBulkDelete, handleEvict, handleEditYaml, handleShowDependencies, showDetails, handlePortForward]);

    return (
        <>
            <ResourceList loadState={loadState}
                title="Pods"
                columns={view ? viewColumns : columns}
                source={view ?? null}
                viewPinnedColumns={POD_PINNED_COLUMNS}
                data={view ? NO_DATA : pods}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                volatileInputs={metrics}
                initialSort={POD_INITIAL_SORT}
                resourceType="pods"
                onRowClick={view ? showDetails : handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
                bulkCustomActions={bulkPodActions}
                onFilteredUidsChange={handleFilteredUidsChange}
                customHeaderActions={
                    <button
                        onClick={handleBellClick}
                        onContextMenu={handleBellRightClick}
                        className={`p-1.5 rounded transition-colors ${
                            notificationsEnabled
                                ? 'bg-primary/20 text-primary'
                                : 'text-gray-400 hover:text-white hover:bg-white/10'
                        }`}
                        title={notificationsEnabled ? 'Disable pod notifications (⇧+click for settings)' : 'Enable pod notifications (⇧+click for settings)'}
                    >
                        {notificationsEnabled
                            ? <BellAlertIcon className="w-4 h-4" />
                            : <BellSlashIcon className="w-4 h-4" />
                        }
                    </button>
                }
            />

            <NotificationSettingsMenu
                isOpen={notificationSettingsMenu.open}
                onClose={() => setNotificationSettingsMenu(prev => ({ ...prev, open: false }))}
                position={{ x: notificationSettingsMenu.x, y: notificationSettingsMenu.y }}
                throttleSeconds={notificationThrottle}
                onThrottleChange={setNotificationThrottle}
                selectedSound={notificationSound}
                onSoundChange={setNotificationSound}
                onPreviewSound={handlePreviewSound}
            />

            <BulkActionModal
                {...bulkModalProps}
                action={bulkActionModal.action || 'delete'}
                actionLabel={bulkActionModal.action === 'evict' ? 'Evict' : 'Delete'}
                onExportYaml={exportYaml}
            />

            {/* Port Forward Dialog */}
            {portForwardDialog.pod && (
                <PodPortForwardDialog
                    open={portForwardDialog.open}
                    onOpenChange={() => setPortForwardDialog({ open: false, pod: null, port: null })}
                    pod={portForwardDialog.pod}
                    containerPort={portForwardDialog.port}
                    currentContext={currentContext}
                />
            )}

            {/* Port Selection Menu (for pods with multiple ports) */}
            {portSelectMenu.open && (
                <div className="fixed inset-0 z-[100] flex items-center justify-center">
                    <div className="absolute inset-0 bg-black/50" onClick={() => setPortSelectMenu({ open: false, pod: null, ports: [] })} />
                    <div className="relative bg-surface border border-border rounded-lg shadow-xl w-full max-w-xs mx-4 py-2">
                        <div className="px-4 py-2 text-sm font-medium text-gray-300 border-b border-border">
                            Select port to forward
                        </div>
                        {portSelectMenu.ports.map((port: any) => (
                            <button
                                key={`${port.containerName}/${port.containerPort}/${port.protocol || 'TCP'}`}
                                onClick={() => {
                                    setPortSelectMenu({ open: false, pod: null, ports: [] });
                                    setPortForwardDialog({ open: true, pod: portSelectMenu.pod, port });
                                }}
                                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center justify-between"
                            >
                                <span>{port.containerName}/{port.name || port.containerPort}</span>
                                <span className="text-gray-500">{port.containerPort}/{port.protocol || 'TCP'}</span>
                            </button>
                        ))}
                    </div>
                </div>
            )}
        </>
    );
}
