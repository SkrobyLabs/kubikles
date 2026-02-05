import React, { useState, useEffect, useMemo, useRef } from 'react';
import { createPortal } from 'react-dom';
import { Environment } from '../../../wailsjs/runtime/runtime';
import { isInServerMode } from '../../lib/wailsjs-adapter/runtime/runtime';
import appIcon from '../../assets/images/logo-universal.png';
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
    ChevronDownIcon,
    ChevronRightIcon,
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
    BugAntIcon,
    SparklesIcon
} from '@heroicons/react/24/outline';
import { useConfig } from '../../context';
import { useAIChat } from '../../context';
import { usePerformancePanel } from '../../hooks/usePerformancePanel';
import { useDebugLogs } from '../../hooks/useDebugLogs';
import SearchSelect from '../shared/SearchSelect';
import Logger from '../../utils/Logger';
import { ListCRDs, GetVersionInfo } from '../../../wailsjs/go/main/App';

export default function Sidebar({
    activeView,
    onViewChange,
    contexts,
    currentContext,
    onContextChange,
    onContextSelectorOpen
}) {
    const { openConfigEditor } = useConfig();
    const { isOpen: aiOpen, togglePanel: toggleAI, providerAvailable } = useAIChat();
    const { openPerformancePanel } = usePerformancePanel();
    const { toggleDebug } = useDebugLogs();
    const isServerMode = isInServerMode();
    // Settings menu state
    const [settingsMenuOpen, setSettingsMenuOpen] = useState(false);

    // Version info
    const [versionInfo, setVersionInfo] = useState(null);
    const [isMac, setIsMac] = useState(true); // default true to avoid layout shift on macOS

    useEffect(() => {
        GetVersionInfo().then(setVersionInfo).catch(() => {});
        Environment().then(env => setIsMac(env.platform === 'darwin')).catch(() => {});
    }, []);
    const settingsButtonRef = useRef(null);
    const settingsMenuRef = useRef(null);

    // Close settings menu on click outside
    useEffect(() => {
        if (!settingsMenuOpen) return;

        const handleClickOutside = (event) => {
            if (settingsButtonRef.current && !settingsButtonRef.current.contains(event.target) &&
                settingsMenuRef.current && !settingsMenuRef.current.contains(event.target)) {
                setSettingsMenuOpen(false);
            }
        };

        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, [settingsMenuOpen]);

    // Fetch CRDs for dynamic menu (lazy loaded)
    const [crds, setCRDs] = useState([]);
    const [crdsLoading, setCRDsLoading] = useState(false);
    const [crdsLoaded, setCRDsLoaded] = useState(false);
    // Track which CRD groups are expanded (collapsed by default)
    const [expandedCRDGroups, setExpandedCRDGroups] = useState(() => {
        try {
            const saved = localStorage.getItem('kubikles_expanded_crd_groups');
            return saved ? JSON.parse(saved) : {};
        } catch {
            return {};
        }
    });

    // Reset and refetch CRDs when context changes (if section is open)
    useEffect(() => {
        setCRDsLoaded(false);
        setCRDs([]);
        // If Custom Resources is already open, trigger fetch for new context
        if (!collapsedGroups['Custom Resources'] && currentContext) {
            setCRDsLoading(true);
            ListCRDs()
                .then(list => {
                    setCRDs(list || []);
                    setCRDsLoaded(true);
                })
                .catch(err => {
                    console.error("Failed to fetch CRDs for sidebar:", err);
                    setCRDs([]);
                })
                .finally(() => {
                    setCRDsLoading(false);
                });
        }
    }, [currentContext]);

    // Fetch CRDs when Custom Resources is opened (lazy load)
    const fetchCRDs = async () => {
        if (!currentContext || crdsLoaded || crdsLoading) return;

        setCRDsLoading(true);
        try {
            const list = await ListCRDs();
            setCRDs(list || []);
            setCRDsLoaded(true);
        } catch (err) {
            console.error("Failed to fetch CRDs for sidebar:", err);
            setCRDs([]);
        } finally {
            setCRDsLoading(false);
        }
    };

    // Save expanded CRD groups state
    useEffect(() => {
        localStorage.setItem('kubikles_expanded_crd_groups', JSON.stringify(expandedCRDGroups));
    }, [expandedCRDGroups]);

    // Group CRDs by API group
    const crdGroups = useMemo(() => {
        const groups = {};
        for (const crd of crds) {
            const group = crd.spec?.group || 'unknown';
            if (!groups[group]) {
                groups[group] = [];
            }
            // Get storage version
            const versions = crd.spec?.versions || [];
            const storageVersion = versions.find(v => v.storage)?.name || versions[0]?.name || 'v1';

            groups[group].push({
                kind: crd.spec?.names?.kind,
                plural: crd.spec?.names?.plural,
                version: storageVersion,
                namespaced: crd.spec?.scope === 'Namespaced',
                group: group
            });
        }
        // Sort groups alphabetically, and resources within each group
        const sortedGroups = {};
        Object.keys(groups).sort().forEach(key => {
            sortedGroups[key] = groups[key].sort((a, b) => a.kind.localeCompare(b.kind));
        });
        return sortedGroups;
    }, [crds]);

    const toggleCRDGroup = (groupName) => {
        setExpandedCRDGroups(prev => ({
            ...prev,
            [groupName]: !prev[groupName]
        }));
    };

    const menuGroups = [
        {
            title: 'Metrics',
            items: [
                { id: 'metrics-overview', label: 'Overview', icon: ChartBarIcon },
                { id: 'metrics-settings', label: 'Settings', icon: Cog6ToothIcon },
            ]
        },
        {
            title: 'Cluster',
            items: [
                { id: 'nodes', label: 'Nodes', icon: ServerIcon },
                { id: 'namespaces', label: 'Namespaces', icon: FolderIcon },
                { id: 'events', label: 'Events', icon: BellAlertIcon },
                { id: 'priorityclasses', label: 'Priority Classes', icon: BoltIcon },
            ]
        },
        {
            title: 'Workloads',
            items: [
                { id: 'pods', label: 'Pods', icon: CubeIcon },
                { id: 'deployments', label: 'Deployments', icon: RocketLaunchIcon },
                { id: 'statefulsets', label: 'StatefulSets', icon: CircleStackIcon },
                { id: 'daemonsets', label: 'DaemonSets', icon: CpuChipIcon },
                { id: 'replicasets', label: 'ReplicaSets', icon: Square2StackIcon },
                { id: 'jobs', label: 'Jobs', icon: CommandLineIcon },
                { id: 'cronjobs', label: 'CronJobs', icon: ClockIcon },
            ]
        },
        {
            title: 'Config',
            items: [
                { id: 'configmaps', label: 'ConfigMaps', icon: DocumentTextIcon },
                { id: 'secrets', label: 'Secrets', icon: LockClosedIcon },
                { id: 'hpas', label: 'HPAs', icon: ChartBarIcon },
                { id: 'pdbs', label: 'PDBs', icon: ShieldExclamationIcon },
                { id: 'resourcequotas', label: 'Resource Quotas', icon: AdjustmentsHorizontalIcon },
                { id: 'limitranges', label: 'Limit Ranges', icon: ArrowsPointingOutIcon },
                { id: 'leases', label: 'Leases', icon: ClockIcon },
            ]
        },
        {
            title: 'Network',
            items: [
                { id: 'services', label: 'Services', icon: GlobeAltIcon },
                { id: 'endpoints', label: 'Endpoints', icon: QueueListIcon },
                { id: 'endpointslices', label: 'Endpoint Slices', icon: QueueListIcon },
                { id: 'ingresses', label: 'Ingresses', icon: ArrowsRightLeftIcon },
                { id: 'ingressclasses', label: 'Ingress Classes', icon: TagIcon },
                { id: 'networkpolicies', label: 'Network Policies', icon: ShieldCheckIcon },
                { id: 'portforwards', label: 'Port Forwards', icon: SignalIcon },
            ]
        },
        {
            title: 'Storage',
            items: [
                { id: 'pvcs', label: 'PVCs', icon: CircleStackIcon },
                { id: 'pvs', label: 'PVs', icon: ServerStackIcon },
                { id: 'storageclasses', label: 'Storage Classes', icon: ServerIcon },
                { id: 'csidrivers', label: 'CSI Drivers', icon: CpuChipIcon },
                { id: 'csinodes', label: 'CSI Nodes', icon: ServerIcon },
            ]
        },
        {
            title: 'Helm',
            items: [
                { id: 'helmreleases', label: 'Releases', icon: WrenchScrewdriverIcon },
                { id: 'helmrepos', label: 'Chart Sources', icon: ArchiveBoxIcon },
            ]
        },
        {
            title: 'Access Control',
            items: [
                { id: 'serviceaccounts', label: 'Service Accounts', icon: UserIcon },
                { id: 'roles', label: 'Roles', icon: KeyIcon },
                { id: 'clusterroles', label: 'Cluster Roles', icon: KeyIcon },
                { id: 'rolebindings', label: 'Role Bindings', icon: LinkIcon },
                { id: 'clusterrolebindings', label: 'Cluster Role Bindings', icon: LinkIcon },
            ]
        },
        {
            title: 'Admission Control',
            items: [
                { id: 'validatingwebhooks', label: 'Validating Webhooks', icon: ShieldCheckIcon },
                { id: 'mutatingwebhooks', label: 'Mutating Webhooks', icon: FingerPrintIcon },
            ]
        },
        {
            title: 'Diagnostics',
            items: [
                { id: 'flow-timeline', label: 'Flow Timeline', icon: ClockIcon },
                { id: 'multi-log-viewer', label: 'Multi-Pod Logs', icon: DocumentTextIcon },
                { id: 'resource-diff', label: 'Resource Diff', icon: ArrowsRightLeftIcon },
                { id: 'rbac-checker', label: 'RBAC Checker', icon: ShieldCheckIcon },
            ]
        }
    ];

    // Collapsed categories state with localStorage persistence
    // Custom Resources and Diagnostics are collapsed by default
    const [collapsedGroups, setCollapsedGroups] = useState(() => {
        try {
            const saved = localStorage.getItem('kubikles_collapsed_groups');
            if (saved) {
                const parsed = JSON.parse(saved);
                // Ensure Custom Resources defaults to collapsed if not set
                if (parsed['Custom Resources'] === undefined) {
                    parsed['Custom Resources'] = true;
                }
                // Ensure Diagnostics defaults to collapsed if not set
                if (parsed['Diagnostics'] === undefined) {
                    parsed['Diagnostics'] = true;
                }
                return parsed;
            }
            return { 'Custom Resources': true, 'Diagnostics': true };
        } catch {
            return { 'Custom Resources': true, 'Diagnostics': true };
        }
    });

    useEffect(() => {
        localStorage.setItem('kubikles_collapsed_groups', JSON.stringify(collapsedGroups));
    }, [collapsedGroups]);

    const toggleGroup = (groupTitle) => {
        const isCurrentlyCollapsed = collapsedGroups[groupTitle];
        // If opening Custom Resources, trigger lazy load
        if (groupTitle === 'Custom Resources' && isCurrentlyCollapsed) {
            fetchCRDs();
        }
        setCollapsedGroups(prev => ({
            ...prev,
            [groupTitle]: !prev[groupTitle]
        }));
    };

    const handleViewChange = (viewId) => {
        Logger.info("Navigating to view", { view: viewId });
        onViewChange(viewId);
    };

    const handleContextChange = (newContext) => {
        Logger.info("Context change requested from Sidebar", { context: newContext });
        onContextChange(newContext);
    };

    return (
        <div className="w-64 bg-surface border-r border-border flex flex-col h-full relative">
            {/* Header - pl-20 on macOS for traffic light buttons, pl-4 for server mode or non-macOS */}
            <div className={`h-14 shrink-0 flex items-center border-b border-border ${isServerMode ? 'pl-4' : `${isMac ? 'pl-20' : 'pl-4'} titlebar-drag`}`}>
                <div className="flex items-center gap-2 text-primary font-bold text-lg select-none">
                    <img src={appIcon} alt="" className="h-6 w-6 rounded-full" />
                    <span>Kubikles</span>
                </div>
            </div>

            {/* Context Selector */}
            <div className="p-4 border-b border-border shrink-0">
                <label className="text-xs font-semibold text-gray-400 uppercase tracking-wider mb-2 block">
                    Context
                </label>
                <SearchSelect
                    options={contexts}
                    value={currentContext}
                    onChange={handleContextChange}
                    placeholder="Select Context..."
                    onOpen={onContextSelectorOpen}
                    preserveOrder
                />
            </div>

            {/* Navigation */}
            <nav className="flex-1 overflow-y-auto py-4">
                {menuGroups.map((group) => {
                    const isCollapsed = collapsedGroups[group.title];
                    return (
                        <div key={group.title} className="mb-2">
                            <button
                                onClick={() => toggleGroup(group.title)}
                                className="w-full px-4 py-1.5 flex items-center justify-between hover:bg-white/5 transition-colors"
                            >
                                <span className="text-xs font-semibold text-gray-400 uppercase tracking-wider">
                                    {group.title}
                                </span>
                                {isCollapsed ? (
                                    <ChevronRightIcon className="h-3.5 w-3.5 text-gray-400" />
                                ) : (
                                    <ChevronDownIcon className="h-3.5 w-3.5 text-gray-400" />
                                )}
                            </button>
                            {!isCollapsed && (
                                <ul className="space-y-1 px-2 mt-1">
                                    {group.items.map((item) => {
                                        const Icon = item.icon;
                                        return (
                                            <li key={item.id}>
                                                <button
                                                    onClick={() => handleViewChange(item.id)}
                                                    className={`w-full flex items-center gap-3 px-3 py-2 text-sm rounded-md transition-colors ${activeView === item.id
                                                        ? 'bg-primary/10 text-primary font-medium'
                                                        : 'text-gray-400 hover:text-text hover:bg-white/5'
                                                        }`}
                                                >
                                                    <Icon className="h-5 w-5" />
                                                    {item.label}
                                                </button>
                                            </li>
                                        );
                                    })}
                                </ul>
                            )}
                        </div>
                    );
                })}

                {/* Custom Resources Section */}
                <div className="mb-2">
                    <button
                        onClick={() => toggleGroup('Custom Resources')}
                        className="w-full px-4 py-1.5 flex items-center justify-between hover:bg-white/5 transition-colors"
                    >
                        <span className="text-xs font-semibold text-gray-400 uppercase tracking-wider">
                            Custom Resources
                        </span>
                        {collapsedGroups['Custom Resources'] ? (
                            <ChevronRightIcon className="h-3.5 w-3.5 text-gray-400" />
                        ) : (
                            <ChevronDownIcon className="h-3.5 w-3.5 text-gray-400" />
                        )}
                    </button>
                    {!collapsedGroups['Custom Resources'] && (
                        <div className="px-2 mt-1">
                            {/* Definitions link */}
                            <button
                                onClick={() => handleViewChange('crds')}
                                className={`w-full flex items-center gap-3 px-3 py-2 text-sm rounded-md transition-colors ${activeView === 'crds'
                                    ? 'bg-primary/10 text-primary font-medium'
                                    : 'text-gray-400 hover:text-text hover:bg-white/5'
                                    }`}
                            >
                                <PuzzlePieceIcon className="h-5 w-5" />
                                Definitions
                            </button>

                            {/* Loading indicator */}
                            {crdsLoading && (
                                <div className="flex items-center gap-2 px-3 py-2 text-sm text-gray-400">
                                    <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary"></div>
                                    Loading...
                                </div>
                            )}

                            {/* CRD Groups */}
                            {!crdsLoading && Object.keys(crdGroups).map((groupName) => {
                                const isExpanded = expandedCRDGroups[groupName];
                                const resources = crdGroups[groupName];
                                return (
                                    <div key={groupName} className="mt-1">
                                        <button
                                            onClick={() => toggleCRDGroup(groupName)}
                                            className="w-full flex items-center gap-2 px-3 py-1.5 text-xs rounded-md transition-colors text-gray-300 hover:text-white hover:bg-white/5"
                                        >
                                            {isExpanded ? (
                                                <ChevronDownIcon className="h-3 w-3" />
                                            ) : (
                                                <ChevronRightIcon className="h-3 w-3" />
                                            )}
                                            <span className="truncate" title={groupName}>{groupName}</span>
                                        </button>
                                        {isExpanded && (
                                            <ul className="ml-2 space-y-0.5">
                                                {resources.map((res) => {
                                                    const viewId = `cr:${res.group}:${res.version}:${res.plural}:${res.kind}:${res.namespaced}`;
                                                    return (
                                                        <li key={res.kind}>
                                                            <button
                                                                onClick={() => handleViewChange(viewId)}
                                                                className={`w-full flex items-center pl-7 pr-2 py-1.5 text-sm rounded-md transition-colors ${activeView === viewId
                                                                    ? 'bg-primary/10 text-primary font-medium'
                                                                    : 'text-gray-400 hover:text-text hover:bg-white/5'
                                                                    }`}
                                                            >
                                                                {res.kind}
                                                            </button>
                                                        </li>
                                                    );
                                                })}
                                            </ul>
                                        )}
                                    </div>
                                );
                            })}
                        </div>
                    )}
                </div>
            </nav>

            {/* Footer */}
            <div className="p-4 border-t border-border shrink-0">
                <div className="flex items-center justify-center gap-2 relative">
                    {/* Version display */}
                    <span
                        className="text-xs text-gray-500 cursor-default flex items-center gap-1"
                        title={versionInfo?.commit || ''}
                    >
                        {versionInfo?.version || 'dev'}
                        {versionInfo?.isDirty && (
                            <span className="text-amber-500 font-medium" title="Uncommitted changes">m</span>
                        )}
                    </span>
                    <button
                        ref={settingsButtonRef}
                        onClick={() => setSettingsMenuOpen(!settingsMenuOpen)}
                        className={`p-1 rounded transition-colors ${
                            settingsMenuOpen
                                ? 'text-white bg-white/10'
                                : 'text-gray-500 hover:text-white hover:bg-white/10'
                        }`}
                        title="Settings"
                    >
                        <Cog6ToothIcon className="w-4 h-4" />
                    </button>
                    {providerAvailable && (
                        <button
                            onClick={toggleAI}
                            className={`absolute right-0 p-1 rounded transition-colors ${
                                aiOpen
                                    ? 'text-purple-400 bg-purple-500/15'
                                    : 'text-gray-500 hover:text-purple-400 hover:bg-white/10'
                            }`}
                            title="AI Assistant"
                        >
                            <SparklesIcon className="w-4 h-4" />
                        </button>
                    )}

                    {/* Settings Dropdown Menu */}
                    {settingsMenuOpen && createPortal(
                        <div
                            ref={settingsMenuRef}
                            className="fixed w-44 bg-surface-light border border-border rounded-md shadow-lg py-1 z-[100]"
                            style={{
                                bottom: '52px',
                                left: settingsButtonRef.current?.getBoundingClientRect().left - 60 || 0
                            }}
                        >
                            <button
                                onClick={() => {
                                    setSettingsMenuOpen(false);
                                    openConfigEditor();
                                }}
                                className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
                            >
                                <Cog6ToothIcon className="h-4 w-4" />
                                Settings
                            </button>
                            <button
                                onClick={() => {
                                    setSettingsMenuOpen(false);
                                    openPerformancePanel();
                                }}
                                className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
                            >
                                <ChartBarIcon className="h-4 w-4" />
                                <span className="flex-1">Performance</span>
                                <span className="text-xs text-gray-500">Opt+P</span>
                            </button>
                            <button
                                onClick={() => {
                                    setSettingsMenuOpen(false);
                                    toggleDebug();
                                }}
                                className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
                            >
                                <BugAntIcon className="h-4 w-4" />
                                <span className="flex-1">Debug</span>
                            </button>
                        </div>,
                        document.body
                    )}
                </div>
            </div>
        </div>
    );
}
