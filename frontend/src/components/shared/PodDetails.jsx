import React, { useState, useMemo } from 'react';
import { LockClosedIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import PodContainersTab from './PodContainersTab';

const TAB_CONTAINERS = 'containers';

export default function PodDetails({ pod, onClose, tabContext = '' }) {
    const { currentContext } = useK8s();
    const [activeTab, setActiveTab] = useState(TAB_CONTAINERS);

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    const tabs = useMemo(() => [
        { id: TAB_CONTAINERS, label: 'Containers' },
        // Future tabs can be added here:
        // { id: 'events', label: 'Events' },
        // { id: 'volumes', label: 'Volumes' },
    ], []);

    const renderTabContent = () => {
        switch (activeTab) {
            case TAB_CONTAINERS:
                return (
                    <PodContainersTab
                        pod={pod}
                        isStale={isStale}
                    />
                );
            default:
                return null;
        }
    };

    return (
        <div className="flex flex-col h-full bg-[#1e1e1e]">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-red-900/30 border-b border-red-500/50 text-red-400 shrink-0">
                    <LockClosedIcon className="h-5 w-5" />
                    <span className="text-sm">
                        This pod is from context <span className="font-medium">{tabContext}</span>.
                    </span>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {pod.metadata?.namespace}/{pod.metadata?.name}
                    </div>
                    {/* Tab Toggle */}
                    <div className="flex items-center bg-[#2d2d2d] rounded-md p-0.5">
                        {tabs.map((tab) => (
                            <button
                                key={tab.id}
                                onClick={() => setActiveTab(tab.id)}
                                className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                    activeTab === tab.id
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                {tab.label}
                            </button>
                        ))}
                    </div>
                </div>
                <div className="flex items-center gap-2">
                    <button
                        onClick={onClose}
                        className="px-3 py-1.5 text-xs font-medium text-gray-400 hover:text-text hover:bg-white/5 rounded transition-colors"
                    >
                        Close
                    </button>
                </div>
            </div>

            {/* Content Area */}
            <div className="flex-1 overflow-hidden">
                {renderTabContent()}
            </div>
        </div>
    );
}
