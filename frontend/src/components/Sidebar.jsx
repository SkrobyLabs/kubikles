import React from 'react';
import {
    ServerIcon,
    CubeIcon,
    GlobeAltIcon,
    Cog6ToothIcon,
    CpuChipIcon,
    RocketLaunchIcon,
    LockClosedIcon,
    DocumentTextIcon
} from '@heroicons/react/24/outline';

export default function Sidebar({ activeView, onViewChange, contexts, currentContext, onContextChange }) {

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
            title: 'Network',
            items: [
                { id: 'services', label: 'Services', icon: GlobeAltIcon },
            ]
        },
        {
            title: 'Config',
            items: [
                { id: 'configmaps', label: 'ConfigMaps', icon: Cog6ToothIcon },
                { id: 'secrets', label: 'Secrets', icon: LockClosedIcon },
            ]
        }
    ];

    return (
        <div className="w-64 bg-surface border-r border-border flex flex-col h-full">
            {/* Header */}
            <div className="h-14 flex items-center justify-center border-b border-border">
                <h1 className="text-xl font-bold text-text">Kubikles</h1>
            </div>

            {/* Context Selector */}
            <div className="p-4 border-b border-border space-y-4">
                <div>
                    <label className="block text-xs font-medium text-gray-400 mb-1">Context</label>
                    <select
                        className="w-full bg-background border border-border text-text text-sm rounded px-2 py-1 focus:outline-none focus:border-primary"
                        value={currentContext}
                        onChange={(e) => onContextChange(e.target.value)}
                    >
                        {contexts.map((ctx) => (
                            <option key={ctx} value={ctx}>{ctx}</option>
                        ))}
                    </select>
                </div>
            </div>

            {/* Navigation */}
            <nav className="flex-1 overflow-y-auto py-4">
                {menuGroups.map((group) => (
                    <div key={group.title} className="mb-6">
                        <h3 className="px-4 text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">
                            {group.title}
                        </h3>
                        <ul className="space-y-1">
                            {group.items.map((item) => (
                                <li key={item.id}>
                                    <button
                                        onClick={() => onViewChange(item.id)}
                                        className={`w-full flex items-center px-4 py-2 text-sm font-medium transition-colors ${activeView === item.id
                                                ? 'bg-primary/10 text-primary border-r-2 border-primary'
                                                : 'text-gray-400 hover:bg-white/5 hover:text-text'
                                            }`}
                                    >
                                        <item.icon className="h-5 w-5 mr-3" />
                                        {item.label}
                                    </button>
                                </li>
                            ))}
                        </ul>
                    </div>
                ))}
            </nav>

            {/* Footer / Status */}
            <div className="p-4 border-t border-border text-xs text-gray-500 text-center">
                Cluster / {activeView}
            </div>
        </div>
    );
}
