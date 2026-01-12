import React, { useEffect, useRef, useMemo, useState } from 'react';
import { K8sProvider, useK8s } from './context/K8sContext';
import { UIProvider, useUI } from './context/UIContext';
import { DebugProvider } from './context/DebugContext';
import { ConfigProvider, useConfig } from './context/ConfigContext';
import { NotificationProvider } from './context/NotificationContext';
import Sidebar from './components/layout/Sidebar';
import BottomPanel from './components/layout/BottomPanel';
import ToastContainer from './components/shared/ToastContainer';
import PodList from './features/workloads/pods/PodList';
import DeploymentList from './features/workloads/deployments/DeploymentList';
import StatefulSetList from './features/workloads/statefulsets/StatefulSetList';
import DaemonSetList from './features/workloads/daemonsets/DaemonSetList';
import ReplicaSetList from './features/workloads/replicasets/ReplicaSetList';
import NodeList from './features/cluster/nodes/NodeList';
import NamespaceList from './features/cluster/namespaces/NamespaceList';
import EventList from './features/cluster/events/EventList';
import MetricsList from './features/cluster/metrics/MetricsList';
import ServiceList from './features/network/services/ServiceList';
import IngressList from './features/network/ingresses/IngressList';
import IngressClassList from './features/network/ingressclasses/IngressClassList';
import ConfigMapList from './features/config/configmaps/ConfigMapList';
import SecretList from './features/config/secrets/SecretList';
import JobList from './features/workloads/jobs/JobList';
import CronJobList from './features/workloads/cronjobs/CronJobList';
import PVCList from './features/storage/pvc/PVCList';
import PVList from './features/storage/pv/PVList';
import StorageClassList from './features/storage/storageclass/StorageClassList';
import CRDList from './features/customresources/definitions/CRDList';
import CustomResourceList from './features/customresources/instances/CustomResourceList';
import PortForwardList from './features/portforwards/PortForwardList';
import { HelmReleaseList } from './features/helm/releases';
import { HelmRepoList } from './features/helm/repos';
import ServiceAccountList from './features/access-control/serviceaccounts/ServiceAccountList';
import RoleList from './features/access-control/roles/RoleList';
import ClusterRoleList from './features/access-control/clusterroles/ClusterRoleList';
import RoleBindingList from './features/access-control/rolebindings/RoleBindingList';
import ClusterRoleBindingList from './features/access-control/clusterrolebindings/ClusterRoleBindingList';
import NetworkPolicyList from './features/network/networkpolicies/NetworkPolicyList';
import EndpointsList from './features/network/endpoints/EndpointsList';
import EndpointSliceList from './features/network/endpointslices/EndpointSliceList';
import HPAList from './features/config/hpas/HPAList';
import PDBList from './features/config/pdbs/PDBList';
import ResourceQuotaList from './features/config/resourcequotas/ResourceQuotaList';
import LimitRangeList from './features/config/limitranges/LimitRangeList';
import ValidatingWebhookList from './features/cluster/webhooks/ValidatingWebhookList';
import MutatingWebhookList from './features/cluster/webhooks/MutatingWebhookList';
import PriorityClassList from './features/cluster/priorityclasses/PriorityClassList';
import LeaseList from './features/config/leases/LeaseList';
import CSIDriverList from './features/storage/csidrivers/CSIDriverList';
import CSINodeList from './features/storage/csinodes/CSINodeList';
import { useDebugLogs } from './hooks/useDebugLogs';
import { usePerformancePanel } from './hooks/usePerformancePanel.jsx';
import { LogDebug } from '../wailsjs/go/main/App';
import ConfirmModal from './components/shared/ConfirmModal';
import ConfigEditor from './components/shared/ConfigEditor';
import CommandPalette from './components/shared/CommandPalette';
import CreateResourceModal from './components/shared/CreateResourceModal';

// Resource templates by view
const resourceTemplates = {
    pods: (ns) => `apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: ${ns || 'default'}
spec:
  containers:
    - name: main
      image: nginx:latest
      ports:
        - containerPort: 80
`,
    configmaps: (ns) => `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-configmap
  namespace: ${ns || 'default'}
data:
  key1: value1
  key2: value2
`,
    secrets: (ns) => `apiVersion: v1
kind: Secret
metadata:
  name: my-secret
  namespace: ${ns || 'default'}
type: Opaque
stringData:
  username: admin
  password: changeme
`,
    deployments: (ns) => `apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-deployment
  namespace: ${ns || 'default'}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
        - name: main
          image: nginx:latest
          ports:
            - containerPort: 80
`,
    services: (ns) => `apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: ${ns || 'default'}
spec:
  selector:
    app: my-app
  ports:
    - port: 80
      targetPort: 80
  type: ClusterIP
`,
    namespaces: () => `apiVersion: v1
kind: Namespace
metadata:
  name: my-namespace
`,
    jobs: (ns) => `apiVersion: batch/v1
kind: Job
metadata:
  name: my-job
  namespace: ${ns || 'default'}
spec:
  template:
    spec:
      containers:
        - name: job
          image: busybox
          command: ["echo", "Hello"]
      restartPolicy: Never
`,
    cronjobs: (ns) => `apiVersion: batch/v1
kind: CronJob
metadata:
  name: my-cronjob
  namespace: ${ns || 'default'}
spec:
  schedule: "*/5 * * * *"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
            - name: job
              image: busybox
              command: ["echo", "Hello"]
          restartPolicy: Never
`,
    statefulsets: (ns) => `apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: my-statefulset
  namespace: ${ns || 'default'}
spec:
  serviceName: my-statefulset
  replicas: 1
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
        - name: main
          image: nginx:latest
          ports:
            - containerPort: 80
`,
    daemonsets: (ns) => `apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: my-daemonset
  namespace: ${ns || 'default'}
spec:
  selector:
    matchLabels:
      app: my-app
  template:
    metadata:
      labels:
        app: my-app
    spec:
      containers:
        - name: main
          image: nginx:latest
`,
    ingresses: (ns) => `apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: my-ingress
  namespace: ${ns || 'default'}
spec:
  rules:
    - host: example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: my-service
                port:
                  number: 80
`,
    pvcs: (ns) => `apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: my-pvc
  namespace: ${ns || 'default'}
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
`,
    serviceaccounts: (ns) => `apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-serviceaccount
  namespace: ${ns || 'default'}
`,
    roles: (ns) => `apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: my-role
  namespace: ${ns || 'default'}
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
`,
    rolebindings: (ns) => `apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-rolebinding
  namespace: ${ns || 'default'}
subjects:
  - kind: ServiceAccount
    name: my-serviceaccount
    namespace: ${ns || 'default'}
roleRef:
  kind: Role
  name: my-role
  apiGroup: rbac.authorization.k8s.io
`,
    clusterroles: () => `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-clusterrole
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
`,
    clusterrolebindings: () => `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: my-clusterrolebinding
subjects:
  - kind: ServiceAccount
    name: my-serviceaccount
    namespace: default
roleRef:
  kind: ClusterRole
  name: my-clusterrole
  apiGroup: rbac.authorization.k8s.io
`,
    networkpolicies: (ns) => `apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: my-networkpolicy
  namespace: ${ns || 'default'}
spec:
  podSelector:
    matchLabels:
      app: my-app
  policyTypes:
    - Ingress
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app: allowed-app
      ports:
        - protocol: TCP
          port: 80
`,
    hpas: (ns) => `apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: my-hpa
  namespace: ${ns || 'default'}
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: my-deployment
  minReplicas: 1
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 50
`,
    pdbs: (ns) => `apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: my-pdb
  namespace: ${ns || 'default'}
spec:
  minAvailable: 1
  selector:
    matchLabels:
      app: my-app
`,
    resourcequotas: (ns) => `apiVersion: v1
kind: ResourceQuota
metadata:
  name: my-quota
  namespace: ${ns || 'default'}
spec:
  hard:
    pods: "10"
    requests.cpu: "4"
    requests.memory: 8Gi
    limits.cpu: "8"
    limits.memory: 16Gi
`,
    limitranges: (ns) => `apiVersion: v1
kind: LimitRange
metadata:
  name: my-limitrange
  namespace: ${ns || 'default'}
spec:
  limits:
    - default:
        cpu: 500m
        memory: 512Mi
      defaultRequest:
        cpu: 100m
        memory: 128Mi
      type: Container
`,
};

// Get template for current view
const getResourceTemplate = (viewId, namespace) => {
    const templateFn = resourceTemplates[viewId];
    if (templateFn) {
        return templateFn(namespace);
    }
    // No template available - return empty
    return '';
};

// Get modal title for current view
const getCreateModalTitle = (viewId) => {
    const titles = {
        pods: 'Create Pod',
        configmaps: 'Create ConfigMap',
        secrets: 'Create Secret',
        deployments: 'Create Deployment',
        services: 'Create Service',
        namespaces: 'Create Namespace',
        jobs: 'Create Job',
        cronjobs: 'Create CronJob',
        statefulsets: 'Create StatefulSet',
        daemonsets: 'Create DaemonSet',
        ingresses: 'Create Ingress',
        pvcs: 'Create PersistentVolumeClaim',
        serviceaccounts: 'Create ServiceAccount',
        roles: 'Create Role',
        rolebindings: 'Create RoleBinding',
        clusterroles: 'Create ClusterRole',
        clusterrolebindings: 'Create ClusterRoleBinding',
        networkpolicies: 'Create NetworkPolicy',
        hpas: 'Create HorizontalPodAutoscaler',
        pdbs: 'Create PodDisruptionBudget',
        resourcequotas: 'Create ResourceQuota',
        limitranges: 'Create LimitRange',
    };
    return titles[viewId] || 'Create Resource';
};

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
        closeAllStaleTabs,
        reorderTabs,
        isTabStale,
        panelHeight,
        setPanelHeight
    } = useUI();

    const {
        contexts,
        currentContext,
        switchContext,
        refreshContexts,
        refreshNamespaces,
        triggerRefresh,
        currentNamespace
    } = useK8s();

    const { showConfigEditor } = useConfig();
    const { toggleDebug } = useDebugLogs();
    const { openPerformancePanel } = usePerformancePanel();
    const isDragging = useRef(false);
    const [commandPaletteOpen, setCommandPaletteOpen] = useState(false);
    const [showGenericCreateModal, setShowGenericCreateModal] = useState(false);

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

    // Global Keyboard Shortcuts
    useEffect(() => {
        const handleKeyDown = (e) => {
            // Cmd+Shift+P / Ctrl+Shift+P - Command Palette
            if ((e.metaKey || e.ctrlKey) && e.shiftKey && e.key.toLowerCase() === 'p') {
                e.preventDefault();
                setCommandPaletteOpen(true);
                return;
            }

            // Cmd+Option+P / Ctrl+Alt+P - Performance Panel
            if ((e.metaKey || e.ctrlKey) && e.altKey && e.key.toLowerCase() === 'p') {
                e.preventDefault();
                openPerformancePanel();
                return;
            }

            // Cmd+R / Ctrl+R - Refresh
            if ((e.metaKey || e.ctrlKey) && e.key === 'r') {
                e.preventDefault();
                const msg = "Refresh triggered via shortcut";
                console.log(msg);
                LogDebug(msg).catch(err => console.error("Failed to log debug:", err));

                refreshContexts();
                refreshNamespaces();
                triggerRefresh(); // Signal all data hooks to re-fetch
            }
        };

        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [refreshContexts, refreshNamespaces, triggerRefresh, openPerformancePanel]);

    // Parse custom resource view ID: cr:{group}:{version}:{plural}:{kind}:{namespaced}
    const parsedCRView = useMemo(() => {
        if (!activeView?.startsWith('cr:')) return null;
        const parts = activeView.split(':');
        if (parts.length !== 6) return null;
        return {
            group: parts[1],
            version: parts[2],
            resource: parts[3], // plural name
            kind: parts[4],
            namespaced: parts[5] === 'true'
        };
    }, [activeView]);

    const renderContent = () => {
        // Handle custom resource views
        if (parsedCRView) {
            return (
                <CustomResourceList
                    key={activeView} // Force remount when view changes
                    crdInfo={parsedCRView}
                    isVisible={true}
                />
            );
        }

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
            case 'metrics': return <MetricsList isVisible={true} />;
            case 'services': return <ServiceList isVisible={true} />;
            case 'ingresses': return <IngressList isVisible={true} />;
            case 'ingressclasses': return <IngressClassList isVisible={true} />;
            case 'configmaps': return <ConfigMapList isVisible={true} />;
            case 'secrets': return <SecretList isVisible={true} />;
            case 'pvcs': return <PVCList isVisible={true} />;
            case 'pvs': return <PVList isVisible={true} />;
            case 'storageclasses': return <StorageClassList isVisible={true} />;
            case 'crds': return <CRDList isVisible={true} />;
            case 'portforwards': return <PortForwardList isVisible={true} />;
            case 'helmreleases': return <HelmReleaseList isVisible={true} />;
            case 'helmrepos': return <HelmRepoList isVisible={true} />;
            case 'serviceaccounts': return <ServiceAccountList isVisible={true} />;
            case 'roles': return <RoleList isVisible={true} />;
            case 'clusterroles': return <ClusterRoleList isVisible={true} />;
            case 'rolebindings': return <RoleBindingList isVisible={true} />;
            case 'clusterrolebindings': return <ClusterRoleBindingList isVisible={true} />;
            case 'networkpolicies': return <NetworkPolicyList isVisible={true} />;
            case 'endpoints': return <EndpointsList isVisible={true} />;
            case 'endpointslices': return <EndpointSliceList isVisible={true} />;
            case 'hpas': return <HPAList isVisible={true} />;
            case 'pdbs': return <PDBList isVisible={true} />;
            case 'resourcequotas': return <ResourceQuotaList isVisible={true} />;
            case 'limitranges': return <LimitRangeList isVisible={true} />;
            case 'validatingwebhooks': return <ValidatingWebhookList isVisible={true} />;
            case 'mutatingwebhooks': return <MutatingWebhookList isVisible={true} />;
            case 'priorityclasses': return <PriorityClassList isVisible={true} />;
            case 'leases': return <LeaseList isVisible={true} />;
            case 'csidrivers': return <CSIDriverList isVisible={true} />;
            case 'csinodes': return <CSINodeList isVisible={true} />;
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
                    {showConfigEditor ? (
                        <ConfigEditor />
                    ) : (
                        /* Split View Container */
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
                                        onCloseStaleTabs={closeAllStaleTabs}
                                        onReorder={reorderTabs}
                                        isTabStale={isTabStale}
                                        height={`${panelHeight}%`}
                                    />
                                </>
                            )}
                        </div>
                    )}
                </main>
            </div>
            <ConfirmModal />
            <CommandPalette
                isOpen={commandPaletteOpen}
                onClose={() => setCommandPaletteOpen(false)}
                onCreateResource={() => setShowGenericCreateModal(true)}
            />
            <CreateResourceModal
                isOpen={showGenericCreateModal}
                onClose={() => setShowGenericCreateModal(false)}
                onSuccess={triggerRefresh}
                title={getCreateModalTitle(activeView)}
                template={getResourceTemplate(activeView, currentNamespace)}
            />
        </>
    );
}

function App() {
    return (
        <DebugProvider>
            <ConfigProvider>
                <NotificationProvider>
                    <K8sProvider>
                        <UIProvider>
                            <MainLayout />
                            <ToastContainer />
                        </UIProvider>
                    </K8sProvider>
                </NotificationProvider>
            </ConfigProvider>
        </DebugProvider>
    );
}

export default App;
