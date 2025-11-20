
import React, { useState, useEffect, useRef } from 'react';
import Sidebar from './components/Sidebar';
import ResourceList from './components/ResourceList';
import LogViewer from './components/LogViewer';
import BottomPanel from './components/BottomPanel';
import PodActionsMenu from './components/PodActionsMenu';
import YamlEditor from './components/YamlEditor';
import Terminal from './components/Terminal';
import { ListPods, ListNodes, ListServices, ListConfigMaps, ListSecrets, ListDeployments, ListNamespaces, ListContexts, SwitchContext, GetCurrentContext, DeletePod, ForceDeletePod, GetPodYaml, UpdatePodYaml, OpenTerminal } from '../wailsjs/go/main/App';

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

    useEffect(() => {
        if (currentContext) {
            saveContextState(currentContext, activeView, currentNamespace);
        }
    }, [currentContext, activeView, currentNamespace]);

    useEffect(() => {
        fetchData(activeView, currentNamespace);
    }, [activeView, currentNamespace, currentContext]);

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

    const handleDeletePod = async (pod) => {
        const isTerminating = pod.metadata.deletionTimestamp;
        const action = isTerminating ? "Force Delete" : "Delete";
        if (!confirm(`Are you sure you want to ${action} pod ${pod.metadata.name}?`)) return;

        try {
            if (isTerminating) {
                await ForceDeletePod(pod.metadata.namespace, pod.metadata.name);
            } else {
                await DeletePod(pod.metadata.namespace, pod.metadata.name);
            }
            fetchData(activeView, currentNamespace); // Refresh
        } catch (err) {
            console.error(`Failed to ${action} pod`, err);
            alert(`Failed to ${action} pod: ` + err);
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

    const getColumns = (view) => {
        switch (view) {
            case 'pods':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name },
                    { key: 'namespace', label: 'Namespace', render: (item) => item.metadata?.namespace },
                    { key: 'status', label: 'Status', render: (item) => item.status?.phase },
                    { key: 'age', label: 'Age', render: (item) => item.metadata?.creationTimestamp ? new Date(item.metadata.creationTimestamp).toLocaleString() : '' },
                    {
                        key: 'actions', label: 'Actions', render: (item) => (
                            <div className="flex items-center justify-end">
                                <PodActionsMenu
                                    pod={item}
                                    isOpen={activeMenuUid === item.metadata.uid}
                                    onOpenChange={(isOpen) => setActiveMenuUid(isOpen ? item.metadata.uid : null)}
                                    onLogs={() => openLogs(item.metadata.name)}
                                    onEditYaml={openYamlEditor}
                                    onDelete={handleDeletePod}
                                    onShell={handleShell}
                                />
                            </div>
                        )
                    },
                ];
            case 'nodes':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name },
                    { key: 'status', label: 'Status', render: (item) => item.status?.conditions?.find(c => c.type === 'Ready')?.status === 'True' ? 'Ready' : 'NotReady' },
                    { key: 'version', label: 'Version', render: (item) => item.status?.nodeInfo?.kubeletVersion },
                ];
            case 'services':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name },
                    { key: 'type', label: 'Type', render: (item) => item.spec?.type },
                    { key: 'clusterIP', label: 'Cluster IP', render: (item) => item.spec?.clusterIP },
                    { key: 'ports', label: 'Ports', render: (item) => item.spec?.ports?.map(p => `${p.port}/${p.protocol}`).join(', ') || '' },
                ];
            case 'configmaps':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name },
                    { key: 'keys', label: 'Keys', render: (item) => Object.keys(item.data || {}).join(', ') },
                ];
            case 'secrets':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name },
                    { key: 'type', label: 'Type', render: (item) => item.type },
                ];
            case 'deployments':
                return [
                    { key: 'name', label: 'Name', render: (item) => item.metadata?.name },
                    { key: 'ready', label: 'Ready', render: (item) => `${item.status?.readyReplicas || 0}/${item.status?.replicas || 0}` },
                    { key: 'age', label: 'Age', render: (item) => item.metadata?.creationTimestamp ? new Date(item.metadata.creationTimestamp).toLocaleString() : '' },
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

