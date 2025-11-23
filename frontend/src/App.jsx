import React, { useState, useEffect, useRef } from 'react';
import {
    CubeIcon,
    CommandLineIcon,
    TrashIcon,
    ArrowPathIcon,
    EllipsisHorizontalIcon
} from '@heroicons/react/24/outline';
import Sidebar from './components/Sidebar';
import ResourceList from './components/ResourceList';
import LogViewer from './components/LogViewer';
import BottomPanel from './components/BottomPanel';
import PodActionsMenu from './components/PodActionsMenu';
import YamlEditor from './components/YamlEditor';
import DeploymentActionsMenu from './components/DeploymentActionsMenu';
import Terminal from './components/Terminal';
import DebugLogViewer from './components/DebugLogViewer';
import { ListPods, ListNodes, ListServices, ListConfigMaps, ListSecrets, ListDeployments, ListNamespaces, ListContexts, SwitchContext, GetCurrentContext, DeletePod, ForceDeletePod, GetPodYaml, UpdatePodYaml, OpenTerminal, StartPodWatcher, LogDebug, GetDeploymentYaml, UpdateDeploymentYaml, DeleteDeployment, RestartDeployment } from '../wailsjs/go/main/App';

function App() {
    const [activeView, setActiveView] = useState('pods');
    const [data, setData] = useState([]);
    const [loading, setLoading] = useState(false);
    const [podsLoading, setPodsLoading] = useState(false);

    // Namespace State
    const [namespaces, setNamespaces] = useState([]);
    const [currentNamespace, setCurrentNamespace] = useState('default');

    // Context State
    const [contexts, setContexts] = useState([]);
    const [currentContext, setCurrentContext] = useState('');

    // Bottom Panel Tabs State
    const [bottomTabs, setBottomTabs] = useState([]);
    const [activeTabId, setActiveTabId] = useState(null);

    // Resizable Panel State
    const [panelHeight, setPanelHeight] = useState(40); // Percentage
    const isDragging = useRef(false);

    // Data Fetching State
    const fetchIdRef = useRef(0);

    // Menu State
    const [activeMenuId, setActiveMenuId] = useState(null); // Changed from activeMenuId to activeMenuId

    // Persistence Helpers
    const loadContextState = (ctx) => {
        const saved = localStorage.getItem(`kubikles_state_${ctx} `);
        if (saved) {
            try {
                return JSON.parse(saved);
            } catch (e) {
                console.error("Failed to parse saved state", e);
            }
        }
        return { view: 'pods', namespace: 'default' };
    };

    const saveContextState = (ctx, view, ns) => {
        if (!ctx) return;
        localStorage.setItem(`kubikles_state_${ctx} `, JSON.stringify({ view, namespace: ns }));
    };

    useEffect(() => {
        fetchContexts();
        fetchNamespaces();
    }, []);

    // Initial data fetch and context load
    useEffect(() => {
        // Load saved context
        const savedContext = localStorage.getItem('kubikles_context');
        if (savedContext && savedContext !== currentContext) {
            handleContextSwitch(savedContext);
        } else {
            fetchData(activeView, currentNamespace);
        }
    }, []);

    // Save context on change
    useEffect(() => {
        if (currentContext) {
            localStorage.setItem('kubikles_context', currentContext);
        }
    }, [currentContext]);

    useEffect(() => {
        if (currentContext) {
            saveContextState(currentContext, activeView, currentNamespace);
        }
    }, [currentContext, activeView, currentNamespace]);

    useEffect(() => {
        fetchData(activeView, currentNamespace);

        // Start watcher if in pods or deployments view
        if ((activeView === 'pods' || activeView === 'deployments') && currentNamespace) {
            StartPodWatcher(currentNamespace);
        }
    }, [activeView, currentNamespace, currentContext]);

    // Pod Event Listener
    useEffect(() => {
        const handlePodEvent = (event) => {
            if (activeView !== 'pods' && activeView !== 'deployments') return;

            const { type, pod } = event;
            console.log(`Pod Event: ${type} - ${pod.metadata.name} (${pod.status?.phase})`);

            if (activeView === 'pods') {
                setData(prevData => {
                    if (type === 'ADDED') {
                        if (prevData.find(p => p.metadata.uid === pod.metadata.uid)) return prevData;
                        return [...prevData, pod];
                    } else if (type === 'MODIFIED') {
                        return prevData.map(p => p.metadata.uid === pod.metadata.uid ? pod : p);
                    } else if (type === 'DELETED') {
                        return prevData.filter(p => p.metadata.uid !== pod.metadata.uid);
                    }
                    return prevData;
                });
            } else if (activeView === 'deployments') {
                setAllPods(prevPods => {
                    if (type === 'ADDED') {
                        if (prevPods.find(p => p.metadata.uid === pod.metadata.uid)) return prevPods;
                        return [...prevPods, pod];
                    } else if (type === 'MODIFIED') {
                        return prevPods.map(p => p.metadata.uid === pod.metadata.uid ? pod : p);
                    } else if (type === 'DELETED') {
                        return prevPods.filter(p => p.metadata.uid !== pod.metadata.uid);
                    }
                    return prevPods;
                });
            }
        };

        if (window.runtime) {
            window.runtime.EventsOn("pod-event", handlePodEvent);
        }

        return () => {
            if (window.runtime) {
                window.runtime.EventsOff("pod-event");
            }
        };
    }, [activeView]);

    // Global Refresh Shortcut (Cmd+R / Ctrl+R)
    useEffect(() => {
        const handleKeyDown = (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'r') {
                e.preventDefault();
                const msg = "Refresh triggered via shortcut";
                console.log(msg);
                LogDebug(msg).catch(err => console.error("Failed to log debug:", err));

                fetchData(activeView, currentNamespace);
                // Also refresh contexts/namespaces if needed
                fetchContexts();
                fetchNamespaces();
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [activeView, currentNamespace]);

    const fetchContexts = async () => {
        try {
            const list = await ListContexts();
            const curr = await GetCurrentContext();
            // Sort contexts alphabetically
            const sortedList = (list || []).sort((a, b) => a.localeCompare(b));
            setContexts(sortedList);

            // Load saved state
            const savedState = loadContextState(curr);
            setActiveView(savedState.view);
            setCurrentNamespace(savedState.namespace);
            setCurrentContext(curr);
        } catch (err) {
            console.error("Failed to fetch contexts", err);
        }
    };

    const handleContextSwitch = async (newContext) => {
        try {
            await SwitchContext(newContext);

            // Load saved state
            const savedState = loadContextState(newContext);
            setActiveView(savedState.view);
            setCurrentNamespace(savedState.namespace);

            setCurrentContext(newContext);
            await fetchNamespaces();
        } catch (err) {
            console.error("Failed to switch context", err);
        }
    };

    const fetchNamespaces = async () => {
        try {
            const ns = await ListNamespaces();
            setNamespaces(ns ? ns.map(n => n.metadata.name) : []);
        } catch (err) {
            console.error("Failed to fetch namespaces", err);
            setNamespaces([]);
        }
    };

    const [allPods, setAllPods] = useState([]);

    const fetchData = async (view, ns) => {
        const fetchId = ++fetchIdRef.current;
        setLoading(true);
        setData([]);
        try {
            let result = [];
            switch (view) {
                case 'pods':
                    result = await ListPods(ns);
                    break;
                case 'nodes':
                    result = await ListNodes();
                    break;
                case 'services':
                    result = await ListServices(ns);
                    break;
                case 'configmaps':
                    result = await ListConfigMaps(ns);
                    break;
                case 'secrets':
                    result = await ListSecrets(ns);
                    break;
                case 'deployments':
                    setLoading(true);
                    setPodsLoading(true);
                    try {
                        const items = await ListDeployments(ns);
                        setData(items);
                        setLoading(false); // Deployments loaded, main loading off

                        // Load pods in background
                        ListPods(ns).then(pods => {
                            setAllPods(pods || []);
                            setPodsLoading(false);
                        }).catch(err => {
                            console.error("Failed to load pods for deployments:", err);
                            setPodsLoading(false);
                        });
                    } catch (err) {
                        console.error("Failed to fetch deployments:", err);
                        setLoading(false);
                        setPodsLoading(false);
                    }
                    // No 'break;' here as the instruction implies the following block is for other cases.
                    // However, the original code had a break here.
                    // To make it syntactically correct and follow the instruction,
                    // I'll assume the instruction wants the `if (fetchId === fetchIdRef.current)` block
                    // to only apply to cases that set `result` directly.
                    // For 'deployments', `setData` and `setLoading` are handled internally.
                    return; // Exit fetchData early for deployments case
                default:
                    break;
            }
            // Only update if this is the latest request
            if (fetchId === fetchIdRef.current) {
                setData(result || []);
                setLoading(false);
            }
        } catch (err) {
            if (fetchId === fetchIdRef.current) {
                console.error(`Failed to fetch ${view} `, err);
                setLoading(false);
            }
        }
    };

    // ... existing code ...

    const getDeploymentPods = (deployment) => {
        if (!deployment.spec?.selector?.matchLabels) return [];
        const selector = deployment.spec.selector.matchLabels;
        return (allPods || []).filter(pod => {
            if (pod.metadata.namespace !== deployment.metadata.namespace) return false;
            for (const [key, value] of Object.entries(selector)) {
                if (pod.metadata.labels?.[key] !== value) return false;
            }
            return true;
        });
    };

    const getEffectivePodStatus = (pod) => {
        // If pod is terminating, that's the status
        if (pod.metadata?.deletionTimestamp) return 'Terminating';

        const containerStatuses = pod.status?.containerStatuses || [];

        // If multiple containers, ignore Succeeded ones (unless all are succeeded)
        let statusesToCheck = containerStatuses;
        if (containerStatuses.length > 1) {
            const nonSucceeded = containerStatuses.filter(s =>
                !(s.state?.terminated && s.state.terminated.exitCode === 0)
            );
            if (nonSucceeded.length > 0) {
                statusesToCheck = nonSucceeded;
            }
        }

        // Find worst status among relevant containers
        let worstStatus = null;
        let worstPriority = -1;

        // Reuse the severity logic from getPodStatus
        const getStatusSeverity = (s) => {
            switch (s) {
                case 'Failed': return 100;
                case 'Terminating': return 90;
                case 'ErrImagePull': return 80;
                case 'CrashLoopBackOff': return 70;
                case 'ImagePullBackOff': return 60;
                case 'ContainerCreating': return 50;
                case 'Pending': return 40;
                case 'Running': return 30;
                case 'Succeeded': return 20;
                default: return 0;
            }
        };

        for (const status of statusesToCheck) {
            let currentStatus = null;
            if (status.state?.waiting) {
                currentStatus = status.state.waiting.reason;
            } else if (status.state?.terminated && status.state.terminated.exitCode !== 0) {
                currentStatus = 'Failed';
            } else if (status.state?.running) {
                currentStatus = 'Running';
            } else if (status.state?.terminated && status.state.terminated.exitCode === 0) {
                currentStatus = 'Succeeded';
            }

            if (currentStatus) {
                const severity = getStatusSeverity(currentStatus);
                if (severity > worstPriority) {
                    worstPriority = severity;
                    worstStatus = currentStatus;
                }
            }
        }

        return worstStatus || pod.status?.phase || 'Unknown';
    };



    const openLogs = (podName) => {
        const tabId = `logs-${podName}`;
        // Check if tab already exists
        if (!bottomTabs.find(t => t.id === tabId)) {
            const newTab = {
                id: tabId,
                title: `Logs: ${podName} `,
                content: <LogViewer namespace={currentNamespace} pod={podName} />
            };
            setBottomTabs([...bottomTabs, newTab]);
        }
        setActiveTabId(tabId);
    };

    const closeTab = (tabId) => {
        const newTabs = bottomTabs.filter(t => t.id !== tabId);
        setBottomTabs(newTabs);
        if (activeTabId === tabId) {
            // Switch to the last tab if available, or null
            setActiveTabId(newTabs.length > 0 ? newTabs[newTabs.length - 1].id : null);
        }
    };

    // Resizing Logic
    const handleMouseDown = (e) => {
        isDragging.current = true;
        document.addEventListener('mousemove', handleMouseMove);
        document.addEventListener('mouseup', handleMouseUp);
    };

    const handleMouseMove = (e) => {
        if (!isDragging.current) return;
        const windowHeight = window.innerHeight;
        const newHeight = ((windowHeight - e.clientY) / windowHeight) * 100;
        // Limit height between 20% and 80%
        if (newHeight > 20 && newHeight < 80) {
            setPanelHeight(newHeight);
        }
    };

    const handleMouseUp = () => {
        isDragging.current = false;
        document.removeEventListener('mousemove', handleMouseMove);
        document.removeEventListener('mouseup', handleMouseUp);
    };

    // Action Handlers
    const handleEditPodYaml = (pod) => { // Renamed from openYamlEditor to handleEditPodYaml
        const tabId = `yaml-${pod.metadata.uid}`;
        // Check if tab already exists
        if (!bottomTabs.find(t => t.id === tabId)) {
            const newTab = {
                id: tabId,
                title: `YAML: ${pod.metadata.name} `,
                content: (
                    <YamlEditor
                        namespace={pod.metadata.namespace}
                        podName={pod.metadata.name}
                        onClose={() => closeTab(tabId)}
                    />
                )
            };
            setBottomTabs([...bottomTabs, newTab]);
        }
        setActiveTabId(tabId);
    };

    const handleDeletePod = async (namespace, name, isTerminating = false) => { // Updated signature
        const actionType = isTerminating ? 'Force Delete' : 'Delete';
        const msg = `handleDeletePod(${actionType}) called for: ${name}, Namespace: ${namespace}, Context: ${currentContext} `;
        console.log(msg);
        try {
            await LogDebug(msg);
        } catch (e) {
            console.error("Failed to LogDebug", e);
        }

        /*
        if (!confirm(`Are you sure you want to delete pod ${ pod.metadata.name }?`)) {
            console.log("Delete cancelled by user");
            return;
        }
        */
        console.log("Auto-confirmed delete for debugging");

        try {
            console.log("Calling backend DeletePod...");
            if (isTerminating) {
                await ForceDeletePod(currentContext, namespace, name);
            } else {
                await DeletePod(currentContext, namespace, name);
            }
            console.log("Backend DeletePod returned success");
            // fetchData(activeView, currentNamespace); // Removed to rely on watcher events
        } catch (err) {
            const action = isTerminating ? 'force delete' : 'delete';
            const errMsg = `Failed to ${action} pod: ${err} `;
            console.error(errMsg);
            await LogDebug(errMsg);
            alert(errMsg);
        }
    };

    const handleEditDeploymentYaml = (deployment) => {
        const tabId = `yaml-deploy-${deployment.metadata.uid}`;
        if (!bottomTabs.find(t => t.id === tabId)) {
            const newTab = {
                id: tabId,
                title: `YAML: ${deployment.metadata.name} `,
                content: (
                    <YamlEditor
                        namespace={deployment.metadata.namespace}
                        podName={deployment.metadata.name} // Reusing prop name, but logic needs to handle deployment
                        isDeployment={true} // New prop to distinguish
                        onClose={() => closeTab(tabId)}
                    />
                )
            };
            setBottomTabs([...bottomTabs, newTab]);
        }
        setActiveTabId(tabId);
    };

    const handleRestartDeployment = async (deployment) => {
        const msg = `Restarting deployment: ${deployment.metadata.name}`;
        console.log(msg);
        try {
            await LogDebug(msg);
            await RestartDeployment(currentContext, deployment.metadata.namespace, deployment.metadata.name);
            console.log("Restart triggered successfully");
            // fetchData(activeView, currentNamespace); // Removed to rely on watcher events
        } catch (err) {
            console.error("Failed to restart deployment", err);
            alert(`Failed to restart deployment: ${err}`);
        }
    };

    const handleDeleteDeployment = async (deployment) => {
        if (!confirm(`Are you sure you want to delete deployment ${deployment.metadata.name}?`)) return;

        const msg = `Deleting deployment: ${deployment.metadata.name} `;
        console.log(msg);
        try {
            await LogDebug(msg);
            await DeleteDeployment(currentContext, deployment.metadata.namespace, deployment.metadata.name);
            console.log("Delete triggered successfully");
            fetchData(activeView, currentNamespace);
        } catch (err) {
            console.error("Failed to delete deployment", err);
            alert(`Failed to delete deployment: ${err} `);
        }
    };

    const handleShell = async (podName) => { // Updated signature to accept podName directly
        try {
            const url = await OpenTerminal(currentContext, currentNamespace, podName, ""); // Use currentNamespace

            const tabId = `shell-${podName}`; // Use podName for tabId
            // Check if tab already exists
            if (!bottomTabs.find(t => t.id === tabId)) {
                const newTab = {
                    id: tabId,
                    title: `Shell: ${podName} `,
                    content: <Terminal url={url} />
                };
                setBottomTabs([...bottomTabs, newTab]);
            }
            setActiveTabId(tabId);
        } catch (err) {
            console.error("Failed to open shell", err);
            alert("Failed to open shell: " + err);
        }
    };

    // Debug Logs
    const [debugLogs, setDebugLogs] = useState([]);

    const isListenerRegistered = useRef(false);

    useEffect(() => {
        if (window.runtime && !isListenerRegistered.current) {
            console.log("Registering debug-log listener");
            window.runtime.EventsOn("debug-log", (msg) => {
                console.log("Backend Debug Log Received:", msg);
                setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] ${msg} `]);
            });
            isListenerRegistered.current = true;
        } else if (!window.runtime) {
            console.error("window.runtime not available");
        }

        return () => {
            // In StrictMode, we might want to keep the listener or handle cleanup carefully.
            // Since EventsOff removes ALL listeners, we should be careful.
            // For now, we'll rely on the ref to prevent double-registration on re-mount.
            // If we unmount for real, we should clean up.
            console.log("Cleanup called (listener registered:", isListenerRegistered.current, ")");
            if (window.runtime && isListenerRegistered.current) {
                window.runtime.EventsOff("debug-log");
                isListenerRegistered.current = false;
            }
        };
    }, []);

    // Sync debug logs to the tab content whenever they change
    useEffect(() => {
        setBottomTabs(prev => prev.map(tab => {
            if (tab.id === 'debug-logs') {
                return {
                    ...tab,
                    content: (
                        <DebugLogViewer
                            logs={debugLogs}
                            onTestEmit={(type) => {
                                if (type === 'ui') {
                                    setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Test Log(UI Only)`]);
                                } else {
                                    window.go.main.App.TestEmit();
                                }
                            }}
                        />
                    )
                };
            }
            return tab;
        }));
    }, [debugLogs]);

    const handleToggleDebug = () => {
        const debugTabId = 'debug-logs';
        const existingTab = bottomTabs.find(t => t.id === debugTabId);

        if (existingTab) {
            // Focus existing tab
        } else {
            const newTab = {
                id: debugTabId,
                title: 'Debug Logs',
                content: (
                    <DebugLogViewer
                        logs={debugLogs}
                        onTestEmit={(type) => {
                            if (type === 'ui') {
                                setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Test Log(UI Only)`]);
                            } else {
                                window.go.main.App.TestEmit();
                            }
                        }}
                    />
                )
            };
            setBottomTabs(prev => [...prev, newTab]);
        }
        setActiveTabId(debugTabId);
    };


    const getPodStatusPriority = (status) => {
        // User requested Ascending order:
        // Succeeded -> Running -> Pending -> ContainerCreating -> ImagePullBackOff -> CrashLoop -> ErrImagePull -> Terminating -> Failed
        // So we assign lower numbers to the top of the list
        switch (status) {
            case 'Succeeded': return 1;
            case 'Running': return 2;
            case 'Pending': return 3;
            case 'ContainerCreating': return 4;
            case 'ImagePullBackOff': return 5;
            case 'CrashLoopBackOff': return 6;
            case 'ErrImagePull': return 7;
            case 'Terminating': return 8;
            case 'Failed': return 9;
            case 'Unknown': return 10;
            default: return 11;
        }
    };


    const getContainerStatusColor = (status) => {
        if (status.state?.running) return 'bg-success';
        if (status.state?.terminated) {
            return status.state.terminated.exitCode === 0 ? 'bg-success/50' : 'bg-error';
        }
        if (status.state?.waiting) {
            const reason = status.state.waiting.reason;
            if (reason === 'CrashLoopBackOff' || reason === 'ImagePullBackOff' || reason === 'ErrImagePull') {
                return 'bg-red-orange';
            }
            if (reason === 'ContainerCreating') {
                return 'bg-warning';
            }
            return 'bg-warning';
        }
        return 'bg-surface'; // Unknown
    };

    const formatAge = (timestamp) => {
        if (!timestamp) return '';
        const start = new Date(timestamp);
        const now = new Date();
        const diff = Math.floor((now - start) / 1000); // seconds

        if (diff < 60) return `${diff} s`;

        const minutes = Math.floor(diff / 60);
        if (minutes < 60) return `${minutes}m ${diff % 60} s`;

        const hours = Math.floor(minutes / 60);
        if (hours < 24) return `${hours}h ${minutes % 60} m`;

        const days = Math.floor(hours / 24);
        return `${days}d ${hours % 24} h`;
    };

    const getPodStatus = (pod) => {
        if (pod.metadata?.deletionTimestamp) return 'Terminating';

        // Check for specific container states that override the general phase
        // We want to pick the "worst" state if multiple containers have issues
        const containerStatuses = pod.status?.containerStatuses || [];
        let worstStatus = null;
        let worstPriority = -1;

        // Helper to check priority of a specific status string
        // Higher number = "worse" / higher priority to show
        const getStatusSeverity = (s) => {
            switch (s) {
                case 'Failed': return 100;
                case 'Terminating': return 90;
                case 'ErrImagePull': return 80;
                case 'CrashLoopBackOff': return 70;
                case 'ImagePullBackOff': return 60;
                case 'ContainerCreating': return 50;
                case 'Pending': return 40;
                case 'Running': return 30;
                case 'Succeeded': return 20;
                default: return 0;
            }
        };

        // First check container statuses
        for (const status of containerStatuses) {
            let currentStatus = null;
            if (status.state?.waiting) {
                currentStatus = status.state.waiting.reason; // e.g. CrashLoopBackOff
            } else if (status.state?.terminated && status.state.terminated.exitCode !== 0) {
                currentStatus = 'Failed'; // Terminated with error
            }

            if (currentStatus) {
                const severity = getStatusSeverity(currentStatus);
                if (severity > worstPriority) {
                    worstPriority = severity;
                    worstStatus = currentStatus;
                }
            }
        }

        if (worstStatus) return worstStatus;

        return pod.status?.phase || 'Unknown';
    };

    const getPodStatusColor = (status) => {
        switch (status) {
            case 'Running':
                return 'text-success';
            case 'Succeeded':
                return 'text-success/70'; // Dimmed green
            case 'Pending':
            case 'ContainerCreating':
                return 'text-warning'; // Orange
            case 'Terminating':
            case 'CrashLoopBackOff':
            case 'ImagePullBackOff':
            case 'ErrImagePull':
            case 'Unknown':
                return 'text-red-orange'; // Orange-red
            case 'Failed':
                return 'text-error'; // Red
            default:
                return 'text-text';
        }
    };

    const getColumns = (view) => {
        switch (view) {
            case 'pods':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
                    {
                        key: 'containers',
                        label: 'Containers',
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
                        getValue: (item) => getPodStatusPriority(getPodStatus(item))
                    },
                    { key: 'restarts', label: 'Restarts', render: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0, getValue: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0 },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
                    {
                        key: 'actions',
                        label: <EllipsisHorizontalIcon className="h-5 w-5" />,
                        render: (item) => (
                            <PodActionsMenu
                                pod={item}
                                isOpen={activeMenuId === `pod-${item.metadata.uid}`}
                                onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `pod-${item.metadata.uid}` : null)}
                                onLogs={() => openLogs(item.metadata.name)}
                                onShell={() => handleShell(item.metadata.name)}
                                onDelete={() => handleDeletePod(item.metadata.namespace, item.metadata.name)}
                                onEditYaml={() => handleEditPodYaml(item)}
                            />
                        ),
                        isColumnSelector: true,
                        disableSort: true
                    },
                ];
            case 'nodes':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
                    { key: 'status', label: 'Status', render: (item) => item.status?.conditions?.find(c => c.type === 'Ready')?.status === 'True' ? 'Ready' : 'NotReady', getValue: (item) => item.status?.conditions?.find(c => c.type === 'Ready')?.status === 'True' ? 'Ready' : 'NotReady' },
                    { key: 'version', label: 'Version', render: (item) => item.status?.nodeInfo?.kubeletVersion, getValue: (item) => item.status?.nodeInfo?.kubeletVersion },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
                ];
            case 'services':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
                    { key: 'type', label: 'Type', render: (item) => item.spec?.type, getValue: (item) => item.spec?.type },
                    { key: 'clusterIP', label: 'Cluster IP', render: (item) => item.spec?.clusterIP, getValue: (item) => item.spec?.clusterIP },
                    { key: 'ports', label: 'Ports', render: (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '', getValue: (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '' },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
                ];
            case 'configmaps':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
                    { key: 'keys', label: 'Keys', render: (item) => Object.keys(item.data || {}).join(', '), getValue: (item) => Object.keys(item.data || {}).join(', ') },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
                ];
            case 'secrets':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
                    { key: 'type', label: 'Type', render: (item) => item.type, getValue: (item) => item.type },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
                ];
            case 'deployments':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
                    {
                        key: 'pods',
                        label: 'Pods',
                        render: (item) => {
                            if (podsLoading) {
                                // Show placeholders based on replicas count
                                const count = item.spec?.replicas ?? 1;
                                if (count === 0) return null;
                                return (
                                    <div className="flex gap-1">
                                        {Array.from({ length: Math.min(count, 5) }).map((_, i) => (
                                            <div
                                                key={i}
                                                className="w-3 h-3 rounded-sm bg-gray-700 animate-pulse"
                                                title="Loading pods..."
                                            />
                                        ))}
                                        {count > 5 && <span className="text-xs text-gray-500">...</span>}
                                    </div>
                                );
                            }
                            return (
                                <div className="flex gap-1">
                                    {getDeploymentPods(item).map((pod) => {
                                        const status = getEffectivePodStatus(pod);
                                        const colorClass = getPodStatusColor(status).replace('text-', 'bg-');
                                        return (
                                            <div
                                                key={pod.metadata.uid}
                                                className={`w-3 h-3 rounded-sm ${colorClass}`}
                                                title={`${pod.metadata.name}: ${status}`}
                                            />
                                        );
                                    })}
                                </div>
                            );
                        },
                        getValue: (item) => getDeploymentPods(item).length
                    },
                    { key: 'ready', label: 'Ready', render: (item) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: (item) => item.status?.readyReplicas || 0 },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
                    {
                        key: 'actions',
                        label: <EllipsisHorizontalIcon className="h-5 w-5" />,
                        render: (item) => (
                            <DeploymentActionsMenu
                                deployment={item}
                                isOpen={activeMenuId === `deploy-${item.metadata.uid}`}
                                onOpenChange={(isOpen) => setActiveMenuId(isOpen ? `deploy-${item.metadata.uid}` : null)}
                                onEditYaml={() => handleEditDeploymentYaml(item)}
                                onRestart={() => handleRestartDeployment(item)}
                                onDelete={() => handleDeleteDeployment(item)}
                            />
                        ),
                        isColumnSelector: true,
                        disableSort: true
                    },
                ];
            default:
                return [];
        }
    };

    const showNamespaceSelector = activeView !== 'nodes';

    return (
        <div className="flex h-screen bg-background text-text font-sans">
            <Sidebar
                activeView={activeView}
                onViewChange={setActiveView}
                contexts={contexts}
                currentContext={currentContext}
                onContextChange={handleContextSwitch}
                onToggleDebug={handleToggleDebug}
            />
            <main className="flex-1 flex flex-col overflow-hidden">
                {/* Split View Container */}
                <div className="flex-1 flex flex-col overflow-hidden">
                    {/* Top Pane: Resource List */}
                    <div
                        className="flex-1 overflow-hidden"
                        style={{ height: bottomTabs.length > 0 ? `${100 - panelHeight}%` : '100%' }}
                    >
                        <ResourceList
                            title={activeView.charAt(0).toUpperCase() + activeView.slice(1)}
                            columns={getColumns(activeView)}
                            data={data}
                            isLoading={loading}
                            namespaces={namespaces}
                            currentNamespace={currentNamespace}
                            onNamespaceChange={setCurrentNamespace}
                            showNamespaceSelector={showNamespaceSelector}
                            highlightedUid={activeMenuId}
                            initialSort={activeView === 'pods' ? { key: 'age', direction: 'asc' } : null}
                        />
                    </div>

                    {/* Bottom Pane */}
                    {bottomTabs.length > 0 && (
                        <>
                            {/* Drag Handle */}
                            <div
                                className="h-1 bg-border hover:bg-primary cursor-row-resize transition-colors"
                                onMouseDown={handleMouseDown}
                            />
                            <BottomPanel
                                tabs={bottomTabs}
                                activeTabId={activeTabId}
                                onTabChange={setActiveTabId}
                                onTabClose={closeTab}
                                height={`${panelHeight}%`}
                            />
                        </>
                    )}
                </div>
            </main>
        </div>
    );
}

export default App;
