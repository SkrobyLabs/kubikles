import React, { useEffect, useRef } from 'react';
import { K8sProvider, useK8s } from './context/K8sContext';
import { UIProvider, useUI } from './context/UIContext';
import Sidebar from './components/layout/Sidebar';
import BottomPanel from './components/layout/BottomPanel';
import PodList from './features/workloads/pods/PodList';
import DeploymentList from './features/workloads/deployments/DeploymentList';
import StatefulSetList from './features/workloads/statefulsets/StatefulSetList';
import DaemonSetList from './features/workloads/daemonsets/DaemonSetList';
import ReplicaSetList from './features/workloads/replicasets/ReplicaSetList';
import NodeList from './features/cluster/nodes/NodeList';
import NamespaceList from './features/cluster/namespaces/NamespaceList';
import EventList from './features/cluster/events/EventList';
import ServiceList from './features/network/services/ServiceList';
import ConfigMapList from './features/config/configmaps/ConfigMapList';
import SecretList from './features/config/secrets/SecretList';
import JobList from './features/workloads/jobs/JobList';
import CronJobList from './features/workloads/cronjobs/CronJobList';
import PVCList from './features/storage/pvc/PVCList';
import PVList from './features/storage/pv/PVList';
import StorageClassList from './features/storage/storageclass/StorageClassList';
import { useDebugLogs } from './hooks/useDebugLogs';
import { LogDebug } from '../wailsjs/go/main/App';
import ConfirmModal from './components/shared/ConfirmModal';

function MainLayout() {
    const {
        activeView,
        setActiveView,
        bottomTabs,
        activeTabId,
        setActiveTabId,
        closeTab,
        closeOtherTabs,
        closeTabsToRight,
        closeAllTabs,
        reorderTabs,
        panelHeight,
        setPanelHeight
    } = useUI();

    const {
        contexts,
        currentContext,
        switchContext,
        refreshContexts,
        refreshNamespaces,
        currentNamespace
    } = useK8s();

    const { toggleDebug } = useDebugLogs();
    const isDragging = useRef(false);

    // Resizing Logic
    const handleMouseDown = (e) => {
        e.preventDefault();
        isDragging.current = true;
        document.body.style.userSelect = 'none';
        document.body.style.cursor = 'row-resize';
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
        document.body.style.userSelect = '';
        document.body.style.cursor = '';
        document.removeEventListener('mousemove', handleMouseMove);
        document.removeEventListener('mouseup', handleMouseUp);
    };

    // Global Refresh Shortcut (Cmd+R / Ctrl+R)
    useEffect(() => {
        const handleKeyDown = (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'r') {
                e.preventDefault();
                const msg = "Refresh triggered via shortcut";
                console.log(msg);
                LogDebug(msg).catch(err => console.error("Failed to log debug:", err));

                refreshContexts();
                refreshNamespaces();
                // Individual lists will re-fetch if their dependencies change or we can force it?
                // The hooks depend on isVisible. If we want to force refresh, we might need a signal.
                // For now, refreshing contexts/namespaces is a good start. 
                // To refresh data, we might need to expose a refresh function from hooks or toggle a version.
                // But typically React Query or similar handles this. 
                // With our simple hooks, we might need to trigger a re-mount or state change.
                // Let's leave it simple for now: Context/Namespace refresh.
                // If users want data refresh, they can switch views or we can implement a global refresh signal in UIContext later.
                // Actually, simply toggling activeView briefly or something would work but is hacky.
                // A better way is a 'refreshTrigger' in UIContext that increments.
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [refreshContexts, refreshNamespaces]);

    const renderContent = () => {
        switch (activeView) {
            case 'pods': return <PodList isVisible={true} />;
            case 'deployments': return <DeploymentList isVisible={true} />;
            case 'statefulsets': return <StatefulSetList isVisible={true} />;
            case 'daemonsets': return <DaemonSetList isVisible={true} />;
            case 'replicasets': return <ReplicaSetList isVisible={true} />;
            case 'jobs': return <JobList isVisible={true} />;
            case 'cronjobs': return <CronJobList isVisible={true} />;
            case 'nodes': return <NodeList isVisible={true} />;
            case 'namespaces': return <NamespaceList isVisible={true} />;
            case 'events': return <EventList isVisible={true} />;
            case 'services': return <ServiceList isVisible={true} />;
            case 'configmaps': return <ConfigMapList isVisible={true} />;
            case 'secrets': return <SecretList isVisible={true} />;
            case 'pvcs': return <PVCList isVisible={true} />;
            case 'pvs': return <PVList isVisible={true} />;
            case 'storageclasses': return <StorageClassList isVisible={true} />;
            default: return <div className="p-4">Unknown View: {activeView}</div>;
        }
    };

    return (
        <>
            <div className="flex h-screen bg-background text-text font-sans">
                <Sidebar
                    activeView={activeView}
                    onViewChange={setActiveView}
                    contexts={contexts}
                    currentContext={currentContext}
                    onContextChange={switchContext}
                    onToggleDebug={toggleDebug}
                />
                <main className="flex-1 flex flex-col overflow-hidden">
                    {/* Split View Container */}
                    <div className="flex-1 flex flex-col overflow-hidden">
                        {/* Top Pane: Resource List */}
                        <div
                            className="flex-1 overflow-hidden"
                            style={{ height: bottomTabs.length > 0 ? `${100 - panelHeight}%` : '100%' }}
                        >
                            {renderContent()}
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
                                    onCloseOthers={closeOtherTabs}
                                    onCloseToRight={closeTabsToRight}
                                    onCloseAll={closeAllTabs}
                                    onReorder={reorderTabs}
                                    height={`${panelHeight}%`}
                                />
                            </>
                        )}
                    </div>
                </main>
            </div>
            <ConfirmModal />
        </>
    );
}

function App() {
    return (
        <K8sProvider>
            <UIProvider>
                <MainLayout />
            </UIProvider>
        </K8sProvider>
    );
}

export default App;
