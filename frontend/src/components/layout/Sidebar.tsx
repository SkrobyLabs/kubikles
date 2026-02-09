import React, { useState, useEffect, useMemo, useRef } from 'react';
import { createPortal } from 'react-dom';
import { Environment } from 'wailsjs/runtime/runtime';
import { isInServerMode } from '~/lib/wailsjs-adapter/runtime/runtime';
// @ts-ignore
import appIcon from '~/assets/images/logo-universal.png';
import {
    ChevronDownIcon,
    ChevronRightIcon,
    PuzzlePieceIcon,
    Cog6ToothIcon,
    ChartBarIcon,
    BugAntIcon,
    SparklesIcon
} from '@heroicons/react/24/outline';
import { useConfig } from '~/context';
import { useAIChat } from '~/context';
import {
    ALL_MENU_ITEMS,
    DEFAULT_MENU_SECTIONS,
    reconcileLayout,
    type SidebarLayoutSection,
} from '~/constants/menuStructure';
import { usePerformancePanel } from '~/hooks/usePerformancePanel';
import { useDebugLogs } from '~/hooks/useDebugLogs';
import SearchSelect from '../shared/SearchSelect';
import Logger from '~/utils/Logger';
import { ListCRDs, GetVersionInfo } from 'wailsjs/go/main/App';

interface VersionInfo {
    version?: string;
    commit?: string;
    isDirty?: boolean;
}

interface SidebarProps {
    activeView: string;
    onViewChange: (viewId: string) => void;
    contexts: string[];
    currentContext: string;
    onContextChange: (ctx: string) => void;
    onContextSelectorOpen?: () => void;
}

export default function Sidebar({
    activeView,
    onViewChange,
    contexts,
    currentContext,
    onContextChange,
    onContextSelectorOpen
}: SidebarProps) {
    const { openConfigEditor, config, setConfig } = useConfig();
    const { isOpen: aiOpen, togglePanel: toggleAI, providerAvailable } = useAIChat();
    const { openPerformancePanel } = usePerformancePanel();
    const { toggleDebug } = useDebugLogs();
    const isServerMode = isInServerMode();
    // Settings menu state
    const [settingsMenuOpen, setSettingsMenuOpen] = useState(false);

    // Version info
    const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
    const [isMac, setIsMac] = useState(true); // default true to avoid layout shift on macOS

    useEffect(() => {
        GetVersionInfo().then(setVersionInfo).catch(() => {});
        Environment().then((env: any) => setIsMac(env.platform === 'darwin')).catch(() => {});
    }, []);
    const settingsButtonRef = useRef<HTMLButtonElement | null>(null);
    const settingsMenuRef = useRef<HTMLDivElement | null>(null);
    const activeItemRef = useRef<HTMLButtonElement | null>(null);

    // Scroll active menu item into view when it changes
    useEffect(() => {
        if (activeItemRef.current) {
            activeItemRef.current.scrollIntoView({
                behavior: 'smooth',
                block: 'nearest',
            });
        }
    }, [activeView]);

    // Close settings menu on click outside
    useEffect(() => {
        if (!settingsMenuOpen) return;

        const handleClickOutside = (event: MouseEvent) => {
            if (settingsButtonRef.current && !settingsButtonRef.current.contains(event.target as Node) &&
                settingsMenuRef.current && !settingsMenuRef.current.contains(event.target as Node)) {
                setSettingsMenuOpen(false);
            }
        };

        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, [settingsMenuOpen]);

    // Fetch CRDs for dynamic menu (lazy loaded)
    const [crds, setCRDs] = useState<any[]>([]);
    const [crdsLoading, setCRDsLoading] = useState(false);
    const [crdsLoaded, setCRDsLoaded] = useState(false);
    // Track which CRD groups are expanded (collapsed by default)
    const [expandedCRDGroups, setExpandedCRDGroups] = useState<Record<string, boolean>>(() => {
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
                .then((list: any) => {
                    setCRDs(list || []);
                    setCRDsLoaded(true);
                })
                .catch((err: any) => {
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
        } catch (err: any) {
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
        const groups: Record<string, any[]> = {};
        for (const crd of crds) {
            const group = crd.spec?.group || 'unknown';
            if (!groups[group]) {
                groups[group] = [];
            }
            // Get storage version
            const versions = crd.spec?.versions || [];
            const storageVersion = versions.find((v: any) => v.storage)?.name || versions[0]?.name || 'v1';

            groups[group].push({
                kind: crd.spec?.names?.kind,
                plural: crd.spec?.names?.plural,
                version: storageVersion,
                namespaced: crd.spec?.scope === 'Namespaced',
                group: group
            });
        }
        // Sort groups alphabetically, and resources within each group
        const sortedGroups: Record<string, any[]> = {};
        Object.keys(groups).sort().forEach((key: any) => {
            sortedGroups[key] = groups[key].sort((a: any, b: any) => a.kind.localeCompare(b.kind));
        });
        return sortedGroups;
    }, [crds]);

    const toggleCRDGroup = (groupName: string) => {
        setExpandedCRDGroups((prev: Record<string, boolean>) => ({
            ...prev,
            [groupName]: !prev[groupName]
        }));
    };

    // Compute effective sidebar layout from config or defaults
    const sidebarLayout = config?.ui?.sidebar?.layout;
    const excludedItems = config?.ui?.sidebar?.excludedItems;
    const menuGroups = useMemo(() => {
        if (!sidebarLayout) {
            const defaultSections = DEFAULT_MENU_SECTIONS.map((s: any) => ({ id: s.id, title: s.title, items: [...s.items] }));
            // Convert to renderable format with icon/label lookups
            return defaultSections
                .map((section: any) => ({
                    id: section.id,
                    title: section.title,
                    items: section.items
                        .filter((id: any) => ALL_MENU_ITEMS[id])
                        .map((id: any) => {
                            const def = ALL_MENU_ITEMS[id];
                            return { id: def.id, label: def.label, icon: def.icon };
                        }),
                }))
                .filter((group: any) => group.id === 'custom-resources' || group.items.length > 0);
        }

        const { sections, newExcluded } = reconcileLayout(sidebarLayout, excludedItems);
        if (newExcluded.length > 0) {
            // Persist newly discovered defaultHidden items
            setConfig('ui.sidebar.excludedItems', [...(excludedItems ?? []), ...newExcluded]);
        }

        // Convert to renderable format with icon/label lookups
        return sections
            .map((section: any) => ({
                id: section.id,
                title: section.title,
                items: section.items
                    .filter((id: any) => ALL_MENU_ITEMS[id])
                    .map((id: any) => {
                        const def = ALL_MENU_ITEMS[id];
                        return { id: def.id, label: section.itemLabels?.[id] || def.label, icon: def.icon };
                    }),
            }))
            .filter((group: any) => group.id === 'custom-resources' || group.items.length > 0);
    }, [sidebarLayout, excludedItems]);

    // Collapsed categories state with localStorage persistence
    // Custom Resources and Diagnostics are collapsed by default
    const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>(() => {
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

    const toggleGroup = (groupTitle: string) => {
        const isCurrentlyCollapsed = collapsedGroups[groupTitle];
        // If opening Custom Resources, trigger lazy load
        if (groupTitle === 'Custom Resources' && isCurrentlyCollapsed) {
            fetchCRDs();
        }
        setCollapsedGroups((prev: Record<string, boolean>) => ({
            ...prev,
            [groupTitle]: !prev[groupTitle]
        }));
    };

    const handleViewChange = (viewId: string) => {
        Logger.info("Navigating to view", { view: viewId });
        onViewChange(viewId);
    };

    const handleContextChange = (newContext: string) => {
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
                    if (group.id === 'custom-resources') {
                        const isCollapsed = collapsedGroups['Custom Resources'];
                        return (
                            <div key="custom-resources" className="mb-2">
                                <button
                                    onClick={() => toggleGroup('Custom Resources')}
                                    className="w-full px-4 py-1.5 flex items-center justify-between hover:bg-white/5 transition-colors"
                                >
                                    <span className="text-xs font-semibold text-gray-400 uppercase tracking-wider">
                                        Custom Resources
                                    </span>
                                    {isCollapsed ? (
                                        <ChevronRightIcon className="h-3.5 w-3.5 text-gray-400" />
                                    ) : (
                                        <ChevronDownIcon className="h-3.5 w-3.5 text-gray-400" />
                                    )}
                                </button>
                                {!isCollapsed && (
                                    <div className="px-2 mt-1">
                                        <button
                                            ref={activeView === 'crds' ? activeItemRef : undefined}
                                            onClick={() => handleViewChange('crds')}
                                            className={`w-full flex items-center gap-3 px-3 py-2 text-sm rounded-md transition-colors ${activeView === 'crds'
                                                ? 'bg-primary/10 text-primary font-medium'
                                                : 'text-gray-400 hover:text-text hover:bg-white/5'
                                                }`}
                                        >
                                            <PuzzlePieceIcon className="h-5 w-5" />
                                            Definitions
                                        </button>
                                        {crdsLoading && (
                                            <div className="flex items-center gap-2 px-3 py-2 text-sm text-gray-400">
                                                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary"></div>
                                                Loading...
                                            </div>
                                        )}
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
                                                            {resources.map((res: any) => {
                                                                const viewId = `cr:${res.group}:${res.version}:${res.plural}:${res.kind}:${res.namespaced}`;
                                                                return (
                                                                    <li key={res.kind}>
                                                                        <button
                                                                            ref={activeView === viewId ? activeItemRef : undefined}
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
                        );
                    }

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
                                    {group.items.map((item: any) => {
                                        const Icon = item.icon;
                                        return (
                                            <li key={item.id}>
                                                <button
                                                    ref={activeView === item.id ? activeItemRef : undefined}
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
                                left: (settingsButtonRef.current?.getBoundingClientRect()?.left ?? 60) - 60 || 0
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
