import React from 'react';
import {
    CubeIcon,
    ServerIcon,
    GlobeAltIcon,
    DocumentTextIcon,
    LockClosedIcon,
    RocketLaunchIcon
} from '@heroicons/react/24/outline';
import SearchSelect from './SearchSelect';

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
            ]
        },
        {
            title: 'Workloads',
            items: [
                { id: 'pods', label: 'Pods', icon: CubeIcon },
                { id: 'deployments', label: 'Deployments', icon: RocketLaunchIcon },
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

    // Debug Log Trigger
    const [debugClicks, setDebugClicks] = React.useState(0);

    React.useEffect(() => {
        if (debugClicks >= 10) {
            onToggleDebug();
            setDebugClicks(0);
        }
    }, [debugClicks]);

    const handleLogoClick = () => {
        setDebugClicks(prev => prev + 1);
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
                    onChange={onContextChange}
                    placeholder="Select Context..."
                />
            </div>

            {/* Navigation */}
            <nav className="flex-1 overflow-y-auto py-4">
                {menuGroups.map((group) => (
                    <div key={group.title} className="mb-6">
                        <div className="px-4 mb-2">
                            <span className="text-xs font-semibold text-gray-400 uppercase tracking-wider">
                                {group.title}
                            </span>
                        </div>
                        <ul className="space-y-1 px-2">
                            {group.items.map((item) => {
                                const Icon = item.icon;
                                return (
                                    <li key={item.id}>
                                        <button
                                            onClick={() => onViewChange(item.id)}
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
                    </div>
                ))}
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
