
import React, { useState, useEffect, useRef } from 'react';
import Sidebar from './components/Sidebar';
import ResourceList from './components/ResourceList';
import LogViewer from './components/LogViewer';
import BottomPanel from './components/BottomPanel';
import PodActionsMenu from './components/PodActionsMenu';
import YamlEditor from './components/YamlEditor';
import Terminal from './components/Terminal';
import DebugLogViewer from './components/DebugLogViewer';
import { ListPods, ListNodes, ListServices, ListConfigMaps, ListSecrets, ListDeployments, ListNamespaces, ListContexts, SwitchContext, GetCurrentContext, DeletePod, ForceDeletePod, GetPodYaml, UpdatePodYaml, OpenTerminal, StartPodWatcher, LogDebug } from '../wailsjs/go/main/App';

function App() {
    const [activeView, setActiveView] = useState('pods');
    const [data, setData] = useState([]);
    const [loading, setLoading] = useState(false);

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
    const [activeMenuUid, setActiveMenuUid] = useState(null);

    // Persistence Helpers
    const loadContextState = (ctx) => {
        const saved = localStorage.getItem(`kubikles_state_${ctx}`);
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
        localStorage.setItem(`kubikles_state_${ctx}`, JSON.stringify({ view, namespace: ns }));
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

        // Start watcher if in pods view
        if (activeView === 'pods' && currentNamespace) {
            StartPodWatcher(currentNamespace);
        }
    }, [activeView, currentNamespace, currentContext]);

    // Pod Event Listener
    useEffect(() => {
        const handlePodEvent = (event) => {
            if (activeView !== 'pods') return;

            const { type, pod } = event;
            console.log(`Pod Event: ${type} - ${pod.metadata.name} (${pod.status?.phase})`);

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
                    result = await ListDeployments(ns);
                    break;
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
                console.error(`Failed to fetch ${view}`, err);
                setLoading(false);
            }
        }
    };

    const openLogs = (podName) => {
        const tabId = `logs-${podName}`;
        // Check if tab already exists
        if (!bottomTabs.find(t => t.id === tabId)) {
            const newTab = {
                id: tabId,
                title: `Logs: ${podName}`,
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
    const openYamlEditor = (pod) => {
        const tabId = `yaml-${pod.metadata.uid}`;
        // Check if tab already exists
        if (!bottomTabs.find(t => t.id === tabId)) {
            const newTab = {
                id: tabId,
                title: `YAML: ${pod.metadata.name}`,
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

    const handleDeletePod = async (pod, isTerminating = false) => {
        const actionType = isTerminating ? 'Force Delete' : 'Delete';
        const msg = `handleDeletePod (${actionType}) called for: ${pod.metadata.name}, Namespace: ${pod.metadata.namespace}, Context: ${currentContext}`;
        console.log(msg);
        try {
            await LogDebug(msg);
        } catch (e) {
            console.error("Failed to LogDebug", e);
        }

        /*
        if (!confirm(`Are you sure you want to delete pod ${pod.metadata.name}?`)) {
            console.log("Delete cancelled by user");
            return;
        }
        */
        console.log("Auto-confirmed delete for debugging");

        try {
            console.log("Calling backend DeletePod...");
            if (isTerminating) {
                await ForceDeletePod(currentContext, pod.metadata.namespace, pod.metadata.name);
            } else {
                await DeletePod(currentContext, pod.metadata.namespace, pod.metadata.name);
            }
            console.log("Backend DeletePod returned success");
            // fetchData(activeView, currentNamespace); // Removed to rely on watcher events
        } catch (err) {
            const action = isTerminating ? 'force delete' : 'delete';
            const errMsg = `Failed to ${action} pod: ${err}`;
            console.error(errMsg);
            await LogDebug(errMsg);
            alert(errMsg);
        }
    };

    const handleShell = async (pod) => {
        try {
            const url = await OpenTerminal(currentContext, pod.metadata.namespace, pod.metadata.name, "");

            const tabId = `shell-${pod.metadata.uid}`;
            // Check if tab already exists
            if (!bottomTabs.find(t => t.id === tabId)) {
                const newTab = {
                    id: tabId,
                    title: `Shell: ${pod.metadata.name}`,
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
                setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] ${msg}`]);
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
                                    setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Test Log (UI Only)`]);
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
                                setDebugLogs(prev => [...prev, `[${new Date().toLocaleTimeString()}] Test Log (UI Only)`]);
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

    const formatAge = (timestamp) => {
        if (!timestamp) return '';
        const start = new Date(timestamp);
        const now = new Date();
        const diff = Math.floor((now - start) / 1000); // seconds

        if (diff < 60) return `${diff}s`;

        const minutes = Math.floor(diff / 60);
        if (minutes < 60) return `${minutes}m ${diff % 60}s`;

        const hours = Math.floor(minutes / 60);
        if (hours < 24) return `${hours}h ${minutes % 60}m`;

        const days = Math.floor(hours / 24);
        return `${days}d ${hours % 24}h`;
    };

    const getColumns = (view) => {
        switch (view) {
            case 'pods':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name, initialSort: 'asc' },
                    {
                        key: 'status',
                        label: 'Status',
                        render: (item) => item.metadata?.deletionTimestamp ? 'Terminating' : item.status?.phase,
                        getValue: (item) => item.metadata?.deletionTimestamp ? 'Terminating' : item.status?.phase
                    },
                    { key: 'restarts', label: 'Restarts', render: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0, getValue: (item) => item.status?.containerStatuses?.reduce((acc, curr) => acc + curr.restartCount, 0) || 0 },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
                    {
                        key: 'actions', label: 'Actions', render: (item) => (
                            <div className="flex items-center justify-end">
                                <PodActionsMenu
                                    pod={item}
                                    isOpen={activeMenuUid === item.metadata.uid}
                                    onOpenChange={(isOpen) => setActiveMenuUid(isOpen ? item.metadata.uid : null)}
                                    onDelete={() => handleDeletePod(item)}
                                    onForceDelete={() => handleDeletePod(item, true)}
                                    onLogs={() => openLogs(item.metadata.name)}
                                    onEditYaml={() => openYamlEditor(item)}
                                    onShell={() => handleShell(item)}
                                />
                            </div>
                        )
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
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name, getValue: (item) => item.metadata?.name },
                    { key: 'ready', label: 'Ready', render: (item) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}`, getValue: (item) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}` },
                    { key: 'age', label: 'Age', render: (item) => formatAge(item.metadata?.creationTimestamp), getValue: (item) => item.metadata?.creationTimestamp },
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
                            highlightedUid={activeMenuUid}
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
