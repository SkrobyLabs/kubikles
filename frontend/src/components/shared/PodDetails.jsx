import React, { useState, useMemo } from 'react';
import { LockClosedIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../context/K8sContext';
import PodInfoTab from './PodInfoTab';
import PodVolumesTab from './PodVolumesTab';
import PodContainersTab from './PodContainersTab';
import PodEventsTab from './PodEventsTab';

const TAB_BASIC = 'basic';
const TAB_VOLUMES = 'volumes';
const TAB_CONTAINERS = 'containers';
const TAB_EVENTS = 'events';

export default function PodDetails({ pod, tabContext = '' }) {
    const { currentContext } = useK8s();
    const [activeTab, setActiveTab] = useState(TAB_BASIC);

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_VOLUMES, label: 'Volumes' },
        { id: TAB_CONTAINERS, label: 'Containers' },
        { id: TAB_EVENTS, label: 'Events' },
    ], []);

    const renderTabContent = () => {
        switch (activeTab) {
            case TAB_BASIC:
                return (
                    <PodInfoTab
                        pod={pod}
                    />
                );
            case TAB_VOLUMES:
                return (
                    <PodVolumesTab
                        pod={pod}
                    />
                );
            case TAB_CONTAINERS:
                return (
                    <PodContainersTab
                        pod={pod}
                        isStale={isStale}
                    />
                );
            case TAB_EVENTS:
                return (
                    <PodEventsTab
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
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
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
            </div>

            {/* Content Area */}
            <div className="flex-1 overflow-hidden">
                {renderTabContent()}
            </div>
        </div>
    );
}
