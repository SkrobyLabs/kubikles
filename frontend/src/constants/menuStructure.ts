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
    Cog6ToothIcon,
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
} from '@heroicons/react/24/outline';

// ---- Types ----

export interface MenuItemDef {
    id: string;
    label: string;
    icon: React.ComponentType<React.SVGProps<SVGSVGElement>>;
    defaultSection: string;
}

export interface MenuSectionDef {
    id: string;
    title: string;
    items: string[]; // ordered item IDs
}

export interface SidebarLayoutSection {
    id: string;
    title: string;
    items: string[];
    isCustom?: boolean;
    itemLabels?: Record<string, string>;
}

/** Section IDs that cannot be deleted by the user */
export const FIXED_SECTION_IDS = new Set(['custom-resources']);

// ---- Default menu sections (matches original Sidebar.tsx order) ----

export const DEFAULT_MENU_SECTIONS: MenuSectionDef[] = [
    {
        id: 'metrics',
        title: 'Metrics',
        items: ['metrics-overview', 'metrics-settings'],
    },
    {
        id: 'cluster',
        title: 'Cluster',
        items: ['nodes', 'namespaces', 'events', 'priorityclasses'],
    },
    {
        id: 'workloads',
        title: 'Workloads',
        items: ['pods', 'deployments', 'statefulsets', 'daemonsets', 'replicasets', 'jobs', 'cronjobs'],
    },
    {
        id: 'config',
        title: 'Config',
        items: ['configmaps', 'secrets', 'hpas', 'pdbs', 'resourcequotas', 'limitranges', 'leases'],
    },
    {
        id: 'network',
        title: 'Network',
        items: ['services', 'endpoints', 'endpointslices', 'ingresses', 'ingressclasses', 'networkpolicies', 'portforwards'],
    },
    {
        id: 'storage',
        title: 'Storage',
        items: ['pvcs', 'pvs', 'storageclasses', 'csidrivers', 'csinodes'],
    },
    {
        id: 'helm',
        title: 'Helm',
        items: ['helmreleases', 'helmrepos'],
    },
    {
        id: 'access-control',
        title: 'Access Control',
        items: ['serviceaccounts', 'roles', 'clusterroles', 'rolebindings', 'clusterrolebindings'],
    },
    {
        id: 'admission-control',
        title: 'Admission Control',
        items: ['validatingwebhooks', 'mutatingwebhooks'],
    },
    {
        id: 'custom-resources',
        title: 'Custom Resources',
        items: ['crds'],
    },
    {
        id: 'diagnostics',
        title: 'Diagnostics',
        items: ['flow-timeline', 'multi-log-viewer', 'resource-diff', 'rbac-checker'],
    },
];

// ---- All menu items (flat map for lookups) ----

export const ALL_MENU_ITEMS: Record<string, MenuItemDef> = {
    'metrics-overview':      { id: 'metrics-overview',      label: 'Overview',               icon: ChartBarIcon,              defaultSection: 'metrics' },
    'metrics-settings':      { id: 'metrics-settings',      label: 'Settings',               icon: Cog6ToothIcon,             defaultSection: 'metrics' },
    'nodes':                 { id: 'nodes',                 label: 'Nodes',                  icon: ServerIcon,                defaultSection: 'cluster' },
    'namespaces':            { id: 'namespaces',            label: 'Namespaces',             icon: FolderIcon,                defaultSection: 'cluster' },
    'events':                { id: 'events',                label: 'Events',                 icon: BellAlertIcon,             defaultSection: 'cluster' },
    'priorityclasses':       { id: 'priorityclasses',       label: 'Priority Classes',       icon: BoltIcon,                  defaultSection: 'cluster' },
    'pods':                  { id: 'pods',                  label: 'Pods',                   icon: CubeIcon,                  defaultSection: 'workloads' },
    'deployments':           { id: 'deployments',           label: 'Deployments',            icon: RocketLaunchIcon,          defaultSection: 'workloads' },
    'statefulsets':          { id: 'statefulsets',           label: 'StatefulSets',           icon: CircleStackIcon,           defaultSection: 'workloads' },
    'daemonsets':            { id: 'daemonsets',             label: 'DaemonSets',             icon: CpuChipIcon,               defaultSection: 'workloads' },
    'replicasets':           { id: 'replicasets',            label: 'ReplicaSets',            icon: Square2StackIcon,          defaultSection: 'workloads' },
    'jobs':                  { id: 'jobs',                  label: 'Jobs',                   icon: CommandLineIcon,           defaultSection: 'workloads' },
    'cronjobs':              { id: 'cronjobs',              label: 'CronJobs',               icon: ClockIcon,                 defaultSection: 'workloads' },
    'configmaps':            { id: 'configmaps',            label: 'ConfigMaps',             icon: DocumentTextIcon,          defaultSection: 'config' },
    'secrets':               { id: 'secrets',               label: 'Secrets',                icon: LockClosedIcon,            defaultSection: 'config' },
    'hpas':                  { id: 'hpas',                  label: 'HPAs',                   icon: ChartBarIcon,              defaultSection: 'config' },
    'pdbs':                  { id: 'pdbs',                  label: 'PDBs',                   icon: ShieldExclamationIcon,     defaultSection: 'config' },
    'resourcequotas':        { id: 'resourcequotas',        label: 'Resource Quotas',        icon: AdjustmentsHorizontalIcon, defaultSection: 'config' },
    'limitranges':           { id: 'limitranges',           label: 'Limit Ranges',           icon: ArrowsPointingOutIcon,     defaultSection: 'config' },
    'leases':                { id: 'leases',                label: 'Leases',                 icon: ClockIcon,                 defaultSection: 'config' },
    'services':              { id: 'services',              label: 'Services',               icon: GlobeAltIcon,              defaultSection: 'network' },
    'endpoints':             { id: 'endpoints',             label: 'Endpoints',              icon: QueueListIcon,             defaultSection: 'network' },
    'endpointslices':        { id: 'endpointslices',        label: 'Endpoint Slices',        icon: QueueListIcon,             defaultSection: 'network' },
    'ingresses':             { id: 'ingresses',             label: 'Ingresses',              icon: ArrowsRightLeftIcon,       defaultSection: 'network' },
    'ingressclasses':        { id: 'ingressclasses',        label: 'Ingress Classes',        icon: TagIcon,                   defaultSection: 'network' },
    'networkpolicies':       { id: 'networkpolicies',       label: 'Network Policies',       icon: ShieldCheckIcon,           defaultSection: 'network' },
    'portforwards':          { id: 'portforwards',          label: 'Port Forwards',          icon: SignalIcon,                defaultSection: 'network' },
    'pvcs':                  { id: 'pvcs',                  label: 'PVCs',                   icon: CircleStackIcon,           defaultSection: 'storage' },
    'pvs':                   { id: 'pvs',                   label: 'PVs',                    icon: ServerStackIcon,           defaultSection: 'storage' },
    'storageclasses':        { id: 'storageclasses',        label: 'Storage Classes',        icon: ServerIcon,                defaultSection: 'storage' },
    'csidrivers':            { id: 'csidrivers',            label: 'CSI Drivers',            icon: CpuChipIcon,               defaultSection: 'storage' },
    'csinodes':              { id: 'csinodes',              label: 'CSI Nodes',              icon: ServerIcon,                defaultSection: 'storage' },
    'helmreleases':          { id: 'helmreleases',          label: 'Releases',               icon: WrenchScrewdriverIcon,     defaultSection: 'helm' },
    'helmrepos':             { id: 'helmrepos',             label: 'Chart Sources',          icon: ArchiveBoxIcon,            defaultSection: 'helm' },
    'serviceaccounts':       { id: 'serviceaccounts',       label: 'Service Accounts',       icon: UserIcon,                  defaultSection: 'access-control' },
    'roles':                 { id: 'roles',                 label: 'Roles',                  icon: KeyIcon,                   defaultSection: 'access-control' },
    'clusterroles':          { id: 'clusterroles',          label: 'Cluster Roles',          icon: KeyIcon,                   defaultSection: 'access-control' },
    'rolebindings':          { id: 'rolebindings',          label: 'Role Bindings',          icon: LinkIcon,                  defaultSection: 'access-control' },
    'clusterrolebindings':   { id: 'clusterrolebindings',   label: 'Cluster Role Bindings',  icon: LinkIcon,                  defaultSection: 'access-control' },
    'validatingwebhooks':    { id: 'validatingwebhooks',    label: 'Validating Webhooks',    icon: ShieldCheckIcon,           defaultSection: 'admission-control' },
    'mutatingwebhooks':      { id: 'mutatingwebhooks',      label: 'Mutating Webhooks',      icon: FingerPrintIcon,           defaultSection: 'admission-control' },
    'flow-timeline':         { id: 'flow-timeline',         label: 'Flow Timeline',          icon: ClockIcon,                 defaultSection: 'diagnostics' },
    'multi-log-viewer':      { id: 'multi-log-viewer',      label: 'Multi-Pod Logs',         icon: DocumentTextIcon,          defaultSection: 'diagnostics' },
    'resource-diff':         { id: 'resource-diff',         label: 'Resource Diff',          icon: ArrowsRightLeftIcon,       defaultSection: 'diagnostics' },
    'rbac-checker':          { id: 'rbac-checker',          label: 'RBAC Checker',           icon: ShieldCheckIcon,           defaultSection: 'diagnostics' },
    // Custom Resources "Definitions" is a special entry
    'crds':                  { id: 'crds',                  label: 'Definitions',            icon: PuzzlePieceIcon,           defaultSection: 'custom-resources' },
};

// ---- Reconciliation ----

/**
 * Reconcile a stored layout with the current item registry.
 * - Removed items (no longer in registry) are pruned.
 * - Items not in the layout are considered intentionally hidden.
 * - Fixed sections are always present.
 */
export function reconcileLayout(
    storedLayout: SidebarLayoutSection[],
): SidebarLayoutSection[] {
    const result = storedLayout
        .map(section => ({
            ...section,
            items: section.items.filter(id => id in ALL_MENU_ITEMS),
        }))
        .filter(section => section.items.length > 0 || section.isCustom || FIXED_SECTION_IDS.has(section.id));

    // Ensure fixed sections are always present
    for (const fixedId of FIXED_SECTION_IDS) {
        if (!result.find(s => s.id === fixedId)) {
            const defaultSec = DEFAULT_MENU_SECTIONS.find(s => s.id === fixedId);
            if (defaultSec) {
                result.push({
                    id: defaultSec.id,
                    title: defaultSec.title,
                    items: [...defaultSec.items],
                });
            }
        }
    }

    return result;
}

/**
 * Convert DEFAULT_MENU_SECTIONS into SidebarLayoutSection[] format
 * (they're already compatible, just ensure the type)
 */
export function getDefaultLayout(): SidebarLayoutSection[] {
    return DEFAULT_MENU_SECTIONS.map(s => ({
        id: s.id,
        title: s.title,
        items: [...s.items],
    }));
}

/**
 * Get all item IDs that are visible in a layout
 */
export function getVisibleItemIds(layout: SidebarLayoutSection[]): Set<string> {
    const visible = new Set<string>();
    for (const section of layout) {
        for (const itemId of section.items) {
            visible.add(itemId);
        }
    }
    return visible;
}
