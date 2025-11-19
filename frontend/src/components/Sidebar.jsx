import React from 'react';

export default function Sidebar({
    activeView,
    onViewChange,
    contexts,
    currentContext,
    onContextChange,
    namespaces,
    currentNamespace,
    onNamespaceChange
}) {
    const menuGroups = [
        {
            title: 'Cluster',
            items: [
                { id: 'nodes', label: 'Nodes', icon: '💻' },
            ]
        },
        {
            title: 'Workloads',
            items: [
                { id: 'pods', label: 'Pods', icon: '📦' },
                { id: 'deployments', label: 'Deployments', icon: '🚀' },
            ]
        },
        {
            title: 'Network',
            items: [
                { id: 'services', label: 'Services', icon: '🌐' },
            ]
        },
        {
            title: 'Config',
            items: [
                { id: 'configmaps', label: 'ConfigMaps', icon: '⚙️' },
                { id: 'secrets', label: 'Secrets', icon: '🔒' },
            ]
        }
    ];

    return (
        <div className="w-64 bg-surface border-r border-border flex flex-col h-full">
            <div className="p-4 border-b border-border space-y-4">
                <h1 className="text-xl font-bold text-primary">Kubikles</h1>

                {/* Context Selector */}
                <div className="space-y-1">
                    <label className="text-xs text-gray-500 uppercase font-semibold">Context</label>
                    <select
                        value={currentContext}
                        onChange={(e) => onContextChange(e.target.value)}
                        className="w-full bg-background border border-border text-text text-sm rounded px-2 py-1.5 focus:outline-none focus:border-primary"
                    >
                        {contexts.map((ctx) => (
                            <option key={ctx} value={ctx}>{ctx}</option>
                        ))}
                    </select>
                </div>

                {/* Namespace Selector */}
                <div className="space-y-1">
                    <label className="text-xs text-gray-500 uppercase font-semibold">Namespace</label>
                    <select
                        value={currentNamespace}
                        onChange={(e) => onNamespaceChange(e.target.value)}
                        className="w-full bg-background border border-border text-text text-sm rounded px-2 py-1.5 focus:outline-none focus:border-primary"
                    >
                        <option value="default">default</option>
                        {namespaces.map((ns) => (
                            <option key={ns.metadata.uid} value={ns.metadata.name}>
                                {ns.metadata.name}
                            </option>
                        ))}
                    </select>
                </div>
            </div>

            <nav className="flex-1 overflow-y-auto p-2 space-y-6">
                {menuGroups.map((group) => (
                    <div key={group.title}>
                        <h3 className="px-3 text-xs font-semibold text-gray-500 uppercase tracking-wider mb-2">
                            {group.title}
                        </h3>
                        <div className="space-y-1">
                            {group.items.map((item) => (
                                <button
                                    key={item.id}
                                    onClick={() => onViewChange(item.id)}
                                    className={`w-full flex items-center px-3 py-2 text-sm font-medium rounded-md transition-colors ${activeView === item.id
                                            ? 'bg-primary text-white'
                                            : 'text-text hover:bg-background'
                                        }`}
                                >
                                    <span className="mr-3">{item.icon}</span>
                                    {item.label}
                                </button>
                            ))}
                        </div>
                    </div>
                ))}
            </nav>
        </div>
    );
}
