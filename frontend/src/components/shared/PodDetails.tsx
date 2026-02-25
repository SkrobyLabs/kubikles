import React, { useState, useMemo, useCallback, useRef, useEffect } from 'react';
import {
    LockClosedIcon,
    DocumentTextIcon,
    PencilSquareIcon,
    ShareIcon,
    SignalIcon,
    FolderIcon,
    CommandLineIcon,
    ClockIcon
} from '@heroicons/react/24/outline';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { usePodActions } from '~/features/workloads/pods/usePodActions';
import { ListPods } from 'wailsjs/go/main/App';
import { getPodController } from '~/utils/k8s-helpers';
import { DeferredLogViewer, ResolvedLogViewerProps } from './log-viewer';
import Tooltip from './Tooltip';
import PodInfoTab from './PodInfoTab';
import PodVolumesTab from './PodVolumesTab';
import PodContainersTab from './PodContainersTab';
import PodEventsTab from './PodEventsTab';
import PodMetricsTab from './PodMetricsTab';
import PodPortForwardDialog from './PodPortForwardDialog';

const TAB_BASIC = 'basic';
const TAB_VOLUMES = 'volumes';
const TAB_CONTAINERS = 'containers';
const TAB_EVENTS = 'events';
const TAB_METRICS = 'metrics';

export default function PodDetails({ pod, tabContext = '' }: any) {
    const { currentContext } = useK8s();
    const { handleShell, handleEditYaml, handleShowDependencies, handleFiles } = usePodActions();
    const { getDetailTab, setDetailTab, openDiagnostic, openTab } = useUI();
    const activeTab = getDetailTab('pod', TAB_BASIC);
    const setActiveTab = (tab: any) => setDetailTab('pod', tab);

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    // Get containers for logs (including init containers)
    const containers = [
        ...(pod.spec?.initContainers || []).map((c: any) => c.name),
        ...(pod.spec?.containers || []).map((c: any) => c.name)
    ];

    // Handle opening logs with sibling pod discovery
    const handleOpenLogs = useCallback(() => {
        const namespace = pod.metadata?.namespace;
        const podName = pod.metadata?.name;

        openTab({
            id: `logs-pod-${podName}`,
            title: podName,
            keepAlive: true,
            content: (
                <DeferredLogViewer
                    resolve={async (): Promise<ResolvedLogViewerProps | null> => {
                        const controller = getPodController(pod);
                        let siblingPods = [podName];
                        let podContainerMap: Record<string, string[]> = { [podName]: containers };
                        let ownerName = '';

                        if (controller) {
                            try {
                                const allPods = await ListPods('', namespace);
                                const siblings = allPods.filter((p: any) => {
                                    const c = getPodController(p);
                                    return c && c.uid === controller.uid;
                                });

                                if (siblings.length > 0) {
                                    siblingPods = siblings.map((p: any) => p.metadata.name);
                                    podContainerMap = {};
                                    for (const p of siblings) {
                                        podContainerMap[p.metadata.name] = [
                                            ...(p.spec?.initContainers || []).map((c: any) => c.name),
                                            ...(p.spec?.containers || []).map((c: any) => c.name)
                                        ];
                                    }
                                    ownerName = controller.name;
                                }
                            } catch (err: any) {
                                console.error('Failed to fetch sibling pods:', err);
                            }
                        }

                        return {
                            namespace,
                            pod: podName,
                            containers,
                            siblingPods,
                            podContainerMap,
                            ownerName,
                            podCreationTime: pod.metadata?.creationTimestamp,
                        };
                    }}
                    tabContext={currentContext}
                />
            ),
            resourceMeta: { kind: 'Pod', name: podName, namespace },
        });
    }, [pod, containers, openTab, currentContext]);

    // Handle opening Flow Timeline for this pod
    const handleFlowTimeline = useCallback(() => {
        openDiagnostic('flow-timeline', {
            resourceType: 'pod',
            namespace: pod.metadata?.namespace,
            name: pod.metadata?.name
        });
    }, [pod, openDiagnostic]);

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_VOLUMES, label: 'Volumes' },
        { id: TAB_CONTAINERS, label: 'Containers' },
        { id: TAB_EVENTS, label: 'Events' },
        { id: TAB_METRICS, label: 'Metrics' },
    ], []);

    // Port forward state
    const [portForwardDialog, setPortForwardDialog] = useState({ open: false, port: null });
    const [portMenuOpen, setPortMenuOpen] = useState(false);
    const portMenuRef = useRef<any>(null);

    // Get all ports from all containers
    const allPorts = useMemo(() => {
        const ports = [];
        const allContainers = [
            ...(pod.spec?.initContainers || []),
            ...(pod.spec?.containers || [])
        ];
        for (const container of allContainers) {
            for (const port of container.ports || []) {
                ports.push({
                    ...port,
                    containerName: container.name
                });
            }
        }
        return ports;
    }, [pod]);

    // Close port menu when clicking outside
    useEffect(() => {
        const handleClickOutside = (e: any) => {
            if (portMenuRef.current && !portMenuRef.current.contains(e.target)) {
                setPortMenuOpen(false);
            }
        };
        if (portMenuOpen) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [portMenuOpen]);

    const handlePortForwardClick = useCallback(() => {
        if (allPorts.length === 0) return;
        if (allPorts.length === 1) {
            // Single port - open dialog directly
            setPortForwardDialog({ open: true, port: allPorts[0] });
        } else {
            // Multiple ports - show menu
            setPortMenuOpen(prev => !prev);
        }
    }, [allPorts]);

    const handleSelectPort = useCallback((port: any) => {
        setPortMenuOpen(false);
        setPortForwardDialog({ open: true, port });
    }, []);

    const handleClosePortForward = useCallback(() => {
        setPortForwardDialog({ open: false, port: null });
    }, []);

    const renderTabContent = () => {
        switch (activeTab) {
            case TAB_BASIC:
                return (
                    <PodInfoTab
                        pod={pod}
                    />
                );
            case TAB_VOLUMES:
                return (
                    <PodVolumesTab
                        pod={pod}
                    />
                );
            case TAB_CONTAINERS:
                return (
                    <PodContainersTab
                        pod={pod}
                        isStale={isStale}
                    />
                );
            case TAB_EVENTS:
                return (
                    <PodEventsTab
                        pod={pod}
                        isStale={isStale}
                    />
                );
            case TAB_METRICS:
                return (
                    <PodMetricsTab
                        pod={pod}
                        isStale={isStale}
                    />
                );
            default:
                return null;
        }
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <LockClosedIcon className="h-5 w-5" />
                    <span className="text-sm">
                        This pod is from context <span className="font-medium">{tabContext}</span>.
                    </span>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400 selectable">
                        {pod.metadata?.namespace}/{pod.metadata?.name}
                    </div>
                    {/* Tab Toggle */}
                    <div className="flex items-center bg-surface-light rounded-md p-0.5">
                        {tabs.map((tab) => (
                            <button
                                key={tab.id}
                                onClick={() => setActiveTab(tab.id)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    activeTab === tab.id
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                {tab.label}
                            </button>
                        ))}
                    </div>
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        {/* Port Forward Button */}
                        <Tooltip content={allPorts.length === 0 ? 'No ports to forward' : 'Port Forward'}>
                            <div className="relative" ref={portMenuRef}>
                                <button
                                    onClick={handlePortForwardClick}
                                    className={`p-1.5 rounded transition-colors ${
                                        allPorts.length === 0
                                            ? 'text-gray-600 cursor-not-allowed'
                                            : 'text-gray-400 hover:text-white hover:bg-white/10'
                                    }`}
                                    disabled={allPorts.length === 0}
                                >
                                    <SignalIcon className="w-4 h-4" />
                                </button>
                                {/* Port Selection Menu */}
                                {portMenuOpen && allPorts.length > 1 && (
                                    <div className="absolute top-full right-0 mt-1 bg-surface border border-border rounded-lg shadow-lg z-50 py-1 min-w-[180px]">
                                        <div className="px-3 py-1.5 text-xs text-gray-500 border-b border-border">
                                            Select port to forward
                                        </div>
                                        {allPorts.map((port: any, idx: number) => (
                                            <button
                                                key={idx}
                                                onClick={() => handleSelectPort(port)}
                                                className="w-full px-3 py-2 text-left text-sm hover:bg-white/5 text-gray-300 flex items-center justify-between"
                                            >
                                                <span>
                                                    {port.containerPort}
                                                    {port.name && <span className="text-gray-500 ml-1">({port.name})</span>}
                                                </span>
                                                <span className="text-xs text-gray-500">{port.containerName}</span>
                                            </button>
                                        ))}
                                    </div>
                                )}
                            </div>
                        </Tooltip>
                        <Tooltip content="View Logs">
                            <button
                                onClick={handleOpenLogs}
                                className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <DocumentTextIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                        <Tooltip content="Shell">
                            <button
                                onClick={() => handleShell(pod)}
                                className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <CommandLineIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                        <Tooltip content="Edit YAML">
                            <button
                                onClick={() => handleEditYaml(pod)}
                                className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <PencilSquareIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                        <Tooltip content="Dependencies">
                            <button
                                onClick={() => handleShowDependencies(pod)}
                                className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <ShareIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                        <Tooltip content="Browse Files">
                            <button
                                onClick={() => handleFiles(pod)}
                                className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            >
                                <FolderIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                        {/* Diagnostic tools */}
                        <div className="h-4 w-px bg-border mx-1" />
                        <Tooltip content="Flow Timeline">
                            <button
                                onClick={handleFlowTimeline}
                                className="p-1.5 text-gray-400 hover:text-blue-400 hover:bg-white/10 rounded transition-colors"
                            >
                                <ClockIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            <div className="flex-1 overflow-hidden">
                {renderTabContent()}
            </div>

            {/* Port Forward Dialog */}
            <PodPortForwardDialog
                open={portForwardDialog.open}
                onOpenChange={handleClosePortForward}
                pod={pod}
                containerPort={portForwardDialog.port}
                currentContext={currentContext}
            />
        </div>
    );
}
