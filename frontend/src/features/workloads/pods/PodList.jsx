import React, { useMemo, useState, useCallback, useEffect } from 'react';
import { EllipsisVerticalIcon, BellAlertIcon, BellSlashIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import AggregateResourceBar from '../../../components/shared/AggregateResourceBar';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import PodPortForwardDialog from '../../../components/shared/PodPortForwardDialog';
import NotificationSettingsMenu from '../../../components/shared/NotificationSettingsMenu';
import PodActionsMenu from './PodActionsMenu';
import { usePods } from '../../../hooks/resources';
import { usePodMetrics } from '../../../hooks/usePodMetrics';
import { usePodActions } from './usePodActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useMenu } from '../../../context/MenuContext';
import { useSelection } from '../../../hooks/useSelection';
import { useBulkActions } from '../../../hooks/useBulkActions';
import { DeletePod, GetPodYaml } from '../../../../wailsjs/go/main/App';
import { formatAge, formatBytes, formatCpu } from '../../../utils/formatting';
import { getPodStatus, getPodStatusColor, getContainerStatusColor, getPodStatusPriority, getPodController } from '../../../utils/k8s-helpers';
import { getOwnerViewId } from '../../../utils/owner-navigation';
import { useMenuPosition } from '../../../hooks/useMenuPosition';
import { usePodNotifications, warmUpAudio, playNotificationSound } from '../../../hooks/usePodNotifications';
import { ALL_PODS } from '../../../components/shared/log-viewer';

export default function PodList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces, crds, ensureCRDsLoaded } = useK8s();
    const { navigateWithSearch } = useUI();

    // Load CRDs for owner reference resolution (lazy load)
    useEffect(() => {
        if (isVisible) {
            ensureCRDsLoaded();
        }
    }, [isVisible, ensureCRDsLoaded]);
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const selection = useSelection();

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction,
        confirmBulkAction,
        exportYaml,
    } = useBulkActions({
        resourceLabel: 'Pod',
        resourceType: 'pods',
        isNamespaced: true,
        deleteApi: DeletePod,
        getYamlApi: GetPodYaml,
        currentContext,
    });
    const { pods, loading } = usePods(currentContext, selectedNamespaces, isVisible);
    // Delay metrics fetch until pods are loaded to prioritize showing pod list first
    const { metrics, available: metricsAvailable } = usePodMetrics(isVisible, !loading && pods.length > 0);
    const { openLogs, handleShell, handleFiles, handleEditYaml, handleShowDependencies, handleShowDetails } = usePodActions();

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

    const [filteredUids, setFilteredUids] = useState(null);
    const handleFilteredUidsChange = useCallback((uids) => setFilteredUids(uids), []);
    usePodNotifications(pods, notificationsEnabled, filteredUids, {
        soundKey: notificationSound,
        throttleSeconds: notificationThrottle,
    });

    const handleBellClick = useCallback((e) => {
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

    const handleBellRightClick = useCallback((e) => {
        e.preventDefault();
        setNotificationSettingsMenu({ open: true, x: e.clientX, y: e.clientY });
    }, []);

    const handlePreviewSound = useCallback((soundKey) => {
        warmUpAudio();
        playNotificationSound(soundKey);
    }, []);

    // Port forward state
    const [portForwardDialog, setPortForwardDialog] = useState({ open: false, pod: null, port: null });
    const [portSelectMenu, setPortSelectMenu] = useState({ open: false, pod: null, ports: [] });

    const handlePortForward = useCallback((pod) => {
        const allPorts = [];
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

    const columns = useMemo(() => [
        { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
        { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace, getValue: (item) => item.metadata?.namespace },
        {
            key: 'cpu',
            label: 'CPU',
            filterType: 'numeric',
            numericHint: 'CPU usage % (0-100)',
            numericUnit: '%',
            render: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                const m = metrics[key];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.cpuPercent}
                        reservedPercent={m.cpuReservedPercent}
                        committedPercent={m.cpuCommittedPercent}
                        type="cpu"
                        label="CPU"
                        usageValue={m.cpuUsage}
                        reservedValue={m.cpuRequested}
                        committedValue={m.cpuCommitted}
                        capacityValue={m.nodeCpuCapacity}
                        formatValue={formatCpu}
                    />
                );
            },
            getValue: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                return metrics[key]?.cpuCommittedPercent ?? -1;
            },
            getNumericValue: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                return metrics[key]?.cpuPercent ?? NaN;
            }
        },
        {
            key: 'memory',
            label: 'Memory',
            filterType: 'numeric',
            numericHint: 'Memory usage % (0-100)',
            numericUnit: '%',
            render: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                const m = metrics[key];
                if (metricsAvailable === false) return <span className="text-gray-500 italic text-xs">N/A</span>;
                if (!m) return <span className="text-gray-500 text-xs">--</span>;
                return (
                    <AggregateResourceBar
                        usagePercent={m.memPercent}
                        reservedPercent={m.memReservedPercent}
                        committedPercent={m.memCommittedPercent}
                        type="memory"
                        label="Memory"
                        usageValue={m.memoryUsage}
                        reservedValue={m.memRequested}
                        committedValue={m.memCommitted}
                        capacityValue={m.nodeMemCapacity}
                        formatValue={formatBytes}
                    />
                );
            },
            getValue: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                return metrics[key]?.memCommittedPercent ?? -1;
            },
            getNumericValue: (item) => {
                const key = `${item.metadata?.namespace}/${item.metadata?.name}`;
                return metrics[key]?.memPercent ?? NaN;
            }
        },
        {
            key: 'containers',
            label: 'Containers',
            filterable: false,
            render: (item) => (
                <div className="flex gap-1">
                    {(item.status?.containerStatuses || []).map((status, i) => (
                        <div
                            key={i}
                            className={`w-3 h-3 rounded-sm ${getContainerStatusColor(status)}`}
                            title={`${status.name}: ${Object.keys(status.state || {})[0]} (${status.state?.waiting?.reason || ''})`}
                        />
                    ))}
                </div>
            ),
            getValue: (item) => getPodStatusPriority(getPodStatus(item))
        },
        {
            key: 'status',
            label: 'Status',
            render: (item) => {
                const status = getPodStatus(item);
                const colorClass = getPodStatusColor(status);
                return <span className={`font-medium ${colorClass}`}>{status}</span>;
            },
            getValue: (item) => getPodStatusPriority(getPodStatus(item)),
            getFilterValue: (item) => getPodStatus(item),
        },
        {
            key: 'restarts',
            label: 'Restarts',
            filterType: 'numeric',
            numericHint: 'Total restart count',
            render: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0,
            getValue: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0,
            getNumericValue: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0
        },
        {
            key: 'age',
            label: 'Age',
            filterType: 'numeric',
            numericHint: 'Age in hours',
            numericUnit: 'h',
            render: (item) => formatAge(item.metadata?.creationTimestamp),
            getValue: (item) => item.metadata?.creationTimestamp,
            getNumericValue: (item) => {
                if (!item.metadata?.creationTimestamp) return NaN;
                return (Date.now() - new Date(item.metadata.creationTimestamp).getTime()) / 3600000;
            }
        },
        {
            key: 'controlledBy',
            label: 'Controlled By',
            render: (item) => {
                const controller = getPodController(item);
                if (!controller) {
                    return <span className="text-gray-600">-</span>;
                }

                const viewId = getOwnerViewId(controller, crds);

                if (viewId) {
                    return (
                        <button
                            onClick={(e) => {
                                e.stopPropagation();
                                navigateWithSearch(viewId, `uid:"${controller.uid}"`);
                            }}
                            className="text-primary hover:text-primary/80 hover:underline transition-colors"
                            title={`Go to ${controller.kind}: ${controller.name}`}
                        >
                            {controller.kind}
                        </button>
                    );
                }

                return (
                    <span className="text-gray-400" title={controller.name}>
                        {controller.kind}
                    </span>
                );
            },
            getValue: (item) => getPodController(item)?.kind || ''
        },
        // Hidden by default columns
        {
            key: 'qos',
            label: 'QoS',
            defaultHidden: true,
            render: (item) => item.status?.qosClass || '-',
            getValue: (item) => item.status?.qosClass || '',
        },
        {
            key: 'node',
            label: 'Node',
            defaultHidden: true,
            render: (item) => item.spec?.nodeName || <span className="text-gray-500">-</span>,
            getValue: (item) => item.spec?.nodeName || '',
        },
        {
            key: 'podIP',
            label: 'Pod IP',
            defaultHidden: true,
            render: (item) => item.status?.podIP || <span className="text-gray-500">-</span>,
            getValue: (item) => item.status?.podIP || '',
        },
        {
            key: 'hostIP',
            label: 'Host IP',
            defaultHidden: true,
            render: (item) => item.status?.hostIP || <span className="text-gray-500">-</span>,
            getValue: (item) => item.status?.hostIP || '',
        },
        {
            key: 'serviceAccount',
            label: 'Service Account',
            defaultHidden: true,
            render: (item) => item.spec?.serviceAccountName || 'default',
            getValue: (item) => item.spec?.serviceAccountName || 'default',
        },
        {
            key: 'priorityClass',
            label: 'Priority Class',
            defaultHidden: true,
            render: (item) => item.spec?.priorityClassName || <span className="text-gray-500">-</span>,
            getValue: (item) => item.spec?.priorityClassName || '',
        },
        {
            key: 'image',
            label: 'Image',
            defaultHidden: true,
            render: (item) => {
                const containers = item.spec?.containers || [];
                if (containers.length === 0) return '-';
                if (containers.length === 1) return <span title={containers[0].image}>{containers[0].image?.split('/').pop()}</span>;
                return <span title={containers.map(c => c.image).join('\n')}>{containers.length} images</span>;
            },
            getValue: (item) => item.spec?.containers?.[0]?.image || '',
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <PodActionsMenu
                    pod={item}
                    isOpen={activeMenuId === `pod-${item.metadata.uid}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `pod-${item.metadata.uid}`, buttonElement)}
                    onLogs={() => {
                        const containers = [
                            ...(item.spec?.initContainers || []).map(c => c.name),
                            ...(item.spec?.containers || []).map(c => c.name)
                        ];

                        const controller = getPodController(item);
                        let siblingPods = [];
                        if (controller) {
                            siblingPods = pods
                                .filter(p => {
                                    const c = getPodController(p);
                                    return c && c.uid === controller.uid;
                                })
                                .map(p => p.metadata.name);
                        } else {
                            siblingPods = [item.metadata.name];
                        }

                        openLogs(item.metadata.namespace, item.metadata.name, containers, siblingPods, {}, '', item.metadata.creationTimestamp);
                    }}
                    onLogsAll={() => {
                        const controller = getPodController(item);
                        if (!controller) {
                            const containers = [
                                ...(item.spec?.initContainers || []).map(c => c.name),
                                ...(item.spec?.containers || []).map(c => c.name)
                            ];
                            openLogs(item.metadata.namespace, item.metadata.name, containers, [item.metadata.name], {}, '', item.metadata.creationTimestamp);
                            return;
                        }
                        const siblings = pods.filter(p => {
                            const c = getPodController(p);
                            return c && c.uid === controller.uid;
                        });
                        const siblingNames = siblings.map(p => p.metadata.name);
                        const podContainerMap = {};
                        for (const sibling of siblings) {
                            podContainerMap[sibling.metadata.name] = [
                                ...(sibling.spec?.initContainers || []).map(c => c.name),
                                ...(sibling.spec?.containers || []).map(c => c.name)
                            ];
                        }
                        const containers = podContainerMap[item.metadata.name] || [];
                        openLogs(item.metadata.namespace, ALL_PODS, containers, siblingNames, podContainerMap, controller.name, item.metadata.creationTimestamp);
                    }}
                    onShell={() => handleShell(item)}
                    onFiles={() => handleFiles(item)}
                    onDelete={() => openBulkDelete([item])}
                    onForceDelete={() => openBulkDelete([item])}
                    onEditYaml={() => handleEditYaml(item)}
                    onShowDependencies={() => handleShowDependencies(item)}
                    onShowDetails={() => handleShowDetails(item)}
                    onPortForward={() => handlePortForward(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, openLogs, handleShell, handleFiles, openBulkDelete, handleEditYaml, handleShowDependencies, handleShowDetails, handlePortForward, pods, navigateWithSearch, metrics, metricsAvailable, crds]);

    return (
        <>
            <ResourceList
                title="Pods"
                columns={columns}
                data={pods}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'age', direction: 'desc' }}
                resourceType="pods"
                onRowClick={handleShowDetails}
                selectable={true}
                selection={selection}
                onBulkDelete={openBulkDelete}
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
                isOpen={bulkActionModal.isOpen}
                onClose={closeBulkAction}
                action="delete"
                actionLabel="Delete"
                items={bulkActionModal.items}
                onConfirm={confirmBulkAction}
                onExportYaml={exportYaml}
                progress={bulkProgress}
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
                        {portSelectMenu.ports.map((port, idx) => (
                            <button
                                key={idx}
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
