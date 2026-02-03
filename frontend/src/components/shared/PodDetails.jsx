import React, { useMemo, useCallback } from 'react';
import {
    LockClosedIcon,
    DocumentTextIcon,
    PencilSquareIcon,
    ShareIcon,
    FolderIcon,
    CommandLineIcon,
    ClockIcon,
    ShieldCheckIcon,
    DocumentDuplicateIcon
} from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import { useUI } from '../../context/UIContext';
import { usePodActions } from '../../features/workloads/pods/usePodActions';
import { ListPods } from '../../../wailsjs/go/main/App';
import { getPodController } from '../../utils/k8s-helpers';
import Tooltip from './Tooltip';
import PodInfoTab from './PodInfoTab';
import PodVolumesTab from './PodVolumesTab';
import PodContainersTab from './PodContainersTab';
import PodEventsTab from './PodEventsTab';
import PodMetricsTab from './PodMetricsTab';

const TAB_BASIC = 'basic';
const TAB_VOLUMES = 'volumes';
const TAB_CONTAINERS = 'containers';
const TAB_EVENTS = 'events';
const TAB_METRICS = 'metrics';

export default function PodDetails({ pod, tabContext = '' }) {
    const { currentContext } = useK8s();
    const { openLogs, handleShell, handleEditYaml, handleShowDependencies, handleFiles } = usePodActions();
    const { getDetailTab, setDetailTab, openDiagnostic } = useUI();
    const activeTab = getDetailTab('pod', TAB_BASIC);
    const setActiveTab = (tab) => setDetailTab('pod', tab);

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    // Get containers for logs (including init containers)
    const containers = [
        ...(pod.spec?.initContainers || []).map(c => c.name),
        ...(pod.spec?.containers || []).map(c => c.name)
    ];

    // Handle opening logs with sibling pod discovery
    const handleOpenLogs = useCallback(async () => {
        const namespace = pod.metadata?.namespace;
        const controller = getPodController(pod);

        let siblingPods = [pod.metadata?.name];
        let podContainerMap = { [pod.metadata?.name]: containers };
        let ownerName = '';

        if (controller) {
            try {
                const allPods = await ListPods(namespace);
                const siblings = allPods.filter(p => {
                    const c = getPodController(p);
                    return c && c.uid === controller.uid;
                });

                if (siblings.length > 0) {
                    siblingPods = siblings.map(p => p.metadata.name);
                    podContainerMap = {};
                    for (const p of siblings) {
                        podContainerMap[p.metadata.name] = [
                            ...(p.spec?.initContainers || []).map(c => c.name),
                            ...(p.spec?.containers || []).map(c => c.name)
                        ];
                    }
                    ownerName = controller.name;
                }
            } catch (err) {
                console.error('Failed to fetch sibling pods:', err);
            }
        }

        openLogs(
            namespace,
            pod.metadata?.name,
            containers,
            siblingPods,
            podContainerMap,
            ownerName,
            pod.metadata?.creationTimestamp
        );
    }, [pod, containers, openLogs]);

    // Handle opening Flow Timeline for this pod
    const handleFlowTimeline = useCallback(() => {
        openDiagnostic('flow-timeline', {
            resourceType: 'pod',
            namespace: pod.metadata?.namespace,
            name: pod.metadata?.name
        });
    }, [pod, openDiagnostic]);

    // Handle opening Multi-Pod Logs with this pod preselected
    // Filter labels to only meaningful ones for grouping (excluding volatile labels)
    const handleMultiPodLogs = useCallback(() => {
        const controller = getPodController(pod);
        let labelSelector = {};

        if (controller && pod.metadata?.labels) {
            // Labels to exclude - these change per ReplicaSet/revision or are too specific
            const excludeLabels = [
                'pod-template-hash',
                'controller-revision-hash',
                'statefulset.kubernetes.io/pod-name',
                'app.kubernetes.io/version',
                'helm.sh/chart'
            ];

            // Preferred labels for grouping pods (in order of preference)
            const preferredLabels = [
                'app',
                'app.kubernetes.io/name',
                'app.kubernetes.io/instance',
                'app.kubernetes.io/component'
            ];

            const allLabels = pod.metadata.labels;

            // First try to use only preferred labels that exist
            for (const key of preferredLabels) {
                if (allLabels[key]) {
                    labelSelector[key] = allLabels[key];
                }
            }

            // If no preferred labels found, use all labels except excluded ones
            if (Object.keys(labelSelector).length === 0) {
                for (const [key, value] of Object.entries(allLabels)) {
                    if (!excludeLabels.includes(key)) {
                        labelSelector[key] = value;
                    }
                }
            }
        }

        openDiagnostic('multi-log-viewer', {
            initialNamespace: pod.metadata?.namespace,
            initialPodNames: [pod.metadata?.name],
            initialLabelSelector: labelSelector
        });
    }, [pod, openDiagnostic]);

    // Handle RBAC check for this pod's service account
    const handleRBACCheck = useCallback(() => {
        const serviceAccount = pod.spec?.serviceAccountName || 'default';
        openDiagnostic('rbac-checker', {
            initialSubject: {
                kind: 'ServiceAccount',
                name: serviceAccount,
                namespace: pod.metadata?.namespace
            }
        });
    }, [pod, openDiagnostic]);

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_VOLUMES, label: 'Volumes' },
        { id: TAB_CONTAINERS, label: 'Containers' },
        { id: TAB_EVENTS, label: 'Events' },
        { id: TAB_METRICS, label: 'Metrics' },
    ], []);

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
                    <div className="text-sm font-medium text-gray-400">
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
                        <Tooltip content="Multi-Pod Logs">
                            <button
                                onClick={handleMultiPodLogs}
                                className="p-1.5 text-gray-400 hover:text-green-400 hover:bg-white/10 rounded transition-colors"
                            >
                                <DocumentDuplicateIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                        <Tooltip content="Check RBAC">
                            <button
                                onClick={handleRBACCheck}
                                className="p-1.5 text-gray-400 hover:text-amber-400 hover:bg-white/10 rounded transition-colors"
                            >
                                <ShieldCheckIcon className="w-4 h-4" />
                            </button>
                        </Tooltip>
                    </div>
                </div>
            </div>

            {/* Content Area */}
            <div className="flex-1 overflow-hidden">
                {renderTabContent()}
            </div>
        </div>
    );
}
