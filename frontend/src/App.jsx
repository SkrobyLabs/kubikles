
import React, { useState, useEffect } from 'react';
import Sidebar from './components/Sidebar';
import ResourceList from './components/ResourceList';
import LogViewer from './components/LogViewer';
import BottomPanel from './components/BottomPanel';
import { ListPods, ListNodes, ListServices, ListConfigMaps, ListSecrets, ListDeployments, ListNamespaces, ListContexts, SwitchContext, GetCurrentContext } from '../wailsjs/go/main/App';

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

    useEffect(() => {
        fetchContexts();
        fetchNamespaces();
    }, []);

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
            setCurrentContext(curr);
        } catch (err) {
            console.error("Failed to fetch contexts", err);
        }
    };

    const handleContextSwitch = async (newContext) => {
        try {
            await SwitchContext(newContext);
            setCurrentContext(newContext);
            await fetchNamespaces();
            fetchData(activeView, currentNamespace);
        } catch (err) {
            console.error("Failed to switch context", err);
        }
    };

    const fetchNamespaces = async () => {
        try {
            const ns = await ListNamespaces();
            setNamespaces(ns || []);
        } catch (err) {
            console.error("Failed to fetch namespaces", err);
            setNamespaces([]);
        }
    };

    const fetchData = async (view, ns) => {
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
            setData(result || []);
        } catch (err) {
            console.error(`Failed to fetch ${view}`, err);
        } finally {
            setLoading(false);
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
                            <button
                                onClick={(e) => { e.stopPropagation(); openLogs(item.metadata.name); }}
                                className="text-primary hover:underline text-xs"
                            >
                                Logs
                            </button>
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

    return (
        <div className="flex h-screen bg-background text-text font-sans">
            <Sidebar
                activeView={activeView}
                onViewChange={setActiveView}
                contexts={contexts}
                currentContext={currentContext}
                onContextChange={handleContextSwitch}
                namespaces={namespaces}
                currentNamespace={currentNamespace}
                onNamespaceChange={setCurrentNamespace}
            />
            <main className="flex-1 flex flex-col overflow-hidden">
                <header className="h-14 border-b border-border flex items-center px-4 bg-surface shrink-0">
                    <div className="text-sm font-medium text-gray-400">
                        Cluster / <span className="text-text capitalize">{activeView}</span>
                    </div>
                </header>

                {/* Split View Container */}
                <div className="flex-1 flex flex-col overflow-hidden">
                    {/* Top Pane: Resource List */}
                    <div className={`flex-1 overflow-hidden ${bottomTabs.length > 0 ? 'h-[60%]' : 'h-full'}`}>
                        <ResourceList
                            title={activeView.charAt(0).toUpperCase() + activeView.slice(1)}
                            columns={getColumns(activeView)}
                            data={data}
                            isLoading={loading}
                        />
                    </div>

                    {/* Bottom Pane */}
                    {bottomTabs.length > 0 && (
                        <BottomPanel
                            tabs={bottomTabs}
                            activeTabId={activeTabId}
                            onTabChange={setActiveTabId}
                            onTabClose={closeTab}
                            height="40%"
                        />
                    )}
                </div>
            </main>
        </div>
    );
}

export default App;

