// Resource type to icon mapping - centralized registry for consistent icon usage
import {
    CubeIcon,
    ServerIcon,
    GlobeAltIcon,
    DocumentTextIcon,
    LockClosedIcon,
    RocketLaunchIcon,
    ServerStackIcon,
    Square2StackIcon,
    CircleStackIcon,
    CommandLineIcon,
    CpuChipIcon,
    ClockIcon,
    FolderIcon,
    BellAlertIcon,
    PuzzlePieceIcon,
    ArrowsRightLeftIcon,
    TagIcon,
    SignalIcon,
    WrenchScrewdriverIcon,
    ArchiveBoxIcon,
    UserIcon,
    KeyIcon,
    LinkIcon,
    ShieldCheckIcon,
    ArrowsPointingOutIcon,
    ShieldExclamationIcon,
    ChartBarIcon,
    AdjustmentsHorizontalIcon,
    QueueListIcon,
    FingerPrintIcon,
    BoltIcon,
    PencilSquareIcon,
    CodeBracketIcon,
    ShareIcon
} from '@heroicons/react/24/outline';

// Map resource types to their icons
const resourceIconMap = {
    // Cluster
    node: ServerIcon,
    nodes: ServerIcon,
    namespace: FolderIcon,
    namespaces: FolderIcon,
    event: BellAlertIcon,
    events: BellAlertIcon,
    priorityclass: BoltIcon,
    priorityclasses: BoltIcon,
    metrics: ChartBarIcon,

    // Workloads
    pod: CubeIcon,
    pods: CubeIcon,
    deployment: RocketLaunchIcon,
    deployments: RocketLaunchIcon,
    statefulset: CircleStackIcon,
    statefulsets: CircleStackIcon,
    daemonset: CpuChipIcon,
    daemonsets: CpuChipIcon,
    replicaset: Square2StackIcon,
    replicasets: Square2StackIcon,
    job: CommandLineIcon,
    jobs: CommandLineIcon,
    cronjob: ClockIcon,
    cronjobs: ClockIcon,

    // Config
    configmap: DocumentTextIcon,
    configmaps: DocumentTextIcon,
    secret: LockClosedIcon,
    secrets: LockClosedIcon,
    hpa: ChartBarIcon,
    hpas: ChartBarIcon,
    horizontalpodautoscaler: ChartBarIcon,
    pdb: ShieldExclamationIcon,
    pdbs: ShieldExclamationIcon,
    poddisruptionbudget: ShieldExclamationIcon,
    resourcequota: AdjustmentsHorizontalIcon,
    resourcequotas: AdjustmentsHorizontalIcon,
    limitrange: ArrowsPointingOutIcon,
    limitranges: ArrowsPointingOutIcon,
    lease: ClockIcon,
    leases: ClockIcon,

    // Network
    service: GlobeAltIcon,
    services: GlobeAltIcon,
    endpoint: QueueListIcon,
    endpoints: QueueListIcon,
    endpointslice: QueueListIcon,
    endpointslices: QueueListIcon,
    ingress: ArrowsRightLeftIcon,
    ingresses: ArrowsRightLeftIcon,
    ingressclass: TagIcon,
    ingressclasses: TagIcon,
    networkpolicy: ShieldCheckIcon,
    networkpolicies: ShieldCheckIcon,
    portforward: SignalIcon,
    portforwards: SignalIcon,

    // Storage
    pvc: CircleStackIcon,
    pvcs: CircleStackIcon,
    persistentvolumeclaim: CircleStackIcon,
    pv: ServerStackIcon,
    pvs: ServerStackIcon,
    persistentvolume: ServerStackIcon,
    storageclass: ServerIcon,
    storageclasses: ServerIcon,
    csidriver: CpuChipIcon,
    csidrivers: CpuChipIcon,
    csinode: ServerIcon,
    csinodes: ServerIcon,

    // Helm
    helmrelease: WrenchScrewdriverIcon,
    helmreleases: WrenchScrewdriverIcon,
    helmrepo: ArchiveBoxIcon,
    helmrepos: ArchiveBoxIcon,

    // Access Control
    serviceaccount: UserIcon,
    serviceaccounts: UserIcon,
    role: KeyIcon,
    roles: KeyIcon,
    clusterrole: KeyIcon,
    clusterroles: KeyIcon,
    rolebinding: LinkIcon,
    rolebindings: LinkIcon,
    clusterrolebinding: LinkIcon,
    clusterrolebindings: LinkIcon,

    // Admission Control
    validatingwebhook: ShieldCheckIcon,
    validatingwebhooks: ShieldCheckIcon,
    validatingwebhookconfiguration: ShieldCheckIcon,
    mutatingwebhook: FingerPrintIcon,
    mutatingwebhooks: FingerPrintIcon,
    mutatingwebhookconfiguration: FingerPrintIcon,

    // Special tab types
    yaml: PencilSquareIcon,
    edit: PencilSquareIcon,
    deps: ShareIcon,
    dependencies: ShareIcon,
    terminal: CommandLineIcon,
    logs: DocumentTextIcon,

    // CRDs / Custom Resources
    crd: PuzzlePieceIcon,
    customresource: PuzzlePieceIcon,
};

/**
 * Get the icon component for a resource type
 * @param {string} resourceType - The resource type (e.g., 'pod', 'deployment', 'configmap')
 * @returns {React.Component|null} The icon component or null if not found
 */
export function getResourceIcon(resourceType: string) {
    if (!resourceType) return null;
    return (resourceIconMap as Record<string, any>)[resourceType.toLowerCase()] || PuzzlePieceIcon;
}

/**
 * Get icon for a tab based on its ID pattern
 * @param {string} tabId - The tab ID (e.g., 'details-pod-uid123', 'yaml-deployment-uid456')
 * @returns {React.Component|null} The icon component or null if not found
 */
export function getTabIcon(tabId: string) {
    if (!tabId) return null;

    // System tabs that don't need icons (title is always visible)
    if (tabId === 'performance-panel' || tabId === 'debug-logs') {
        return null;
    }

    // Parse tab ID patterns: 'type-resourceType-uid' or 'resourceType-uid'
    const parts = tabId.split('-');
    if (parts.length < 2) return null;

    const prefix = parts[0];

    // Check for special prefixes first (yaml, deps, details)
    if (prefix === 'yaml' || prefix === 'edit') {
        return PencilSquareIcon;
    }
    if (prefix === 'deps') {
        return ShareIcon;
    }
    if (prefix === 'terminal') {
        return CommandLineIcon;
    }
    if (prefix === 'logs') {
        return DocumentTextIcon;
    }

    // For 'details-resourceType-uid' pattern
    if (prefix === 'details' && parts.length >= 3) {
        return getResourceIcon(parts[1]);
    }

    // Fallback: try the prefix itself
    return getResourceIcon(prefix);
}

export default resourceIconMap;
