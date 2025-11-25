import React, { useState, useEffect } from 'react';
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
    ChevronRightIcon
} from '@heroicons/react/24/outline';
import SearchSelect from '../shared/SearchSelect';
import Logger from '../../utils/Logger';

export default function Sidebar({
    activeView,
    onViewChange,
    contexts,
    currentContext,
    onContextChange,
    onToggleDebug
}) {
    const menuGroups = [
        {
            title: 'Cluster',
            items: [
                { id: 'nodes', label: 'Nodes', icon: ServerIcon },
                { id: 'namespaces', label: 'Namespaces', icon: FolderIcon },
                { id: 'events', label: 'Events', icon: BellAlertIcon },
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
            ]
        },
        {
            title: 'Network',
            items: [
                { id: 'services', label: 'Services', icon: GlobeAltIcon },
            ]
        }
    ];

    // Collapsed categories state with localStorage persistence
    const [collapsedGroups, setCollapsedGroups] = useState(() => {
        try {
            const saved = localStorage.getItem('kubikles_collapsed_groups');
            return saved ? JSON.parse(saved) : {};
        } catch {
            return {};
        }
    });

    useEffect(() => {
        localStorage.setItem('kubikles_collapsed_groups', JSON.stringify(collapsedGroups));
    }, [collapsedGroups]);

    const toggleGroup = (groupTitle) => {
        setCollapsedGroups(prev => ({
            ...prev,
            [groupTitle]: !prev[groupTitle]
        }));
    };

    // Debug Log Trigger
    const [debugClicks, setDebugClicks] = useState(0);

    useEffect(() => {
        if (debugClicks >= 10) {
            Logger.info("Toggling Debug Mode via Logo clicks");
            onToggleDebug();
            setDebugClicks(0);
        }
    }, [debugClicks]);

    const handleLogoClick = () => {
        setDebugClicks(prev => prev + 1);
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
            {/* App Header */}
            <div className="h-14 flex items-center px-4 border-b border-border shrink-0">
                <div
                    className="flex items-center gap-2 text-primary font-bold text-xl cursor-pointer select-none"
                    onClick={handleLogoClick}
                >
                    <CubeIcon className="h-6 w-6" />
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
            </nav>

            {/* Footer */}
            <div className="p-4 border-t border-border shrink-0">
                <div className="text-xs text-gray-500 text-center">
                    v0.1.0
                </div>
            </div>
        </div>
    );
}
