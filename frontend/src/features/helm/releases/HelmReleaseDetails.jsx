import React, { useState, useMemo } from 'react';
import { LockClosedIcon, ArrowUturnLeftIcon, TrashIcon } from '@heroicons/react/24/outline';
import { useK8s } from '../../../context/K8sContext';
import HelmReleaseInfoTab from './HelmReleaseInfoTab';
import HelmReleaseHistoryTab from './HelmReleaseHistoryTab';
import HelmReleaseValuesTab from './HelmReleaseValuesTab';
import HelmReleaseEventsTab from './HelmReleaseEventsTab';
import { useHelmReleaseActions } from './useHelmReleaseActions';

const TAB_BASIC = 'basic';
const TAB_HISTORY = 'history';
const TAB_VALUES = 'values';
const TAB_EVENTS = 'events';

export default function HelmReleaseDetails({ release, tabContext = '', initialTab = TAB_BASIC }) {
    const { currentContext } = useK8s();
    const { handleRollback, handleUninstall } = useHelmReleaseActions();
    const [activeTab, setActiveTab] = useState(initialTab);

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    const tabs = useMemo(() => [
        { id: TAB_BASIC, label: 'Basic' },
        { id: TAB_HISTORY, label: 'History' },
        { id: TAB_VALUES, label: 'Values' },
        { id: TAB_EVENTS, label: 'Events' },
    ], []);

    const renderTabContent = () => {
        switch (activeTab) {
            case TAB_BASIC:
                return <HelmReleaseInfoTab release={release} />;
            case TAB_HISTORY:
                return <HelmReleaseHistoryTab release={release} isStale={isStale} />;
            case TAB_VALUES:
                return <HelmReleaseValuesTab release={release} isStale={isStale} />;
            case TAB_EVENTS:
                return <HelmReleaseEventsTab release={release} isStale={isStale} />;
            default:
                return null;
        }
    };

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-red-900/30 border-b border-red-500/50 text-red-400 shrink-0">
                    <LockClosedIcon className="h-5 w-5" />
                    <span className="text-sm">
                        This release is from context <span className="font-medium">{tabContext}</span>.
                    </span>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {release.namespace}/{release.name}
                    </div>
                    {/* Tab Toggle */}
                    <div className="flex items-center bg-surface-light rounded-md p-0.5">
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
                    {/* Action Icons */}
                    <div className="flex items-center gap-1 ml-2">
                        {release.revision > 1 && (
                            <button
                                onClick={() => handleRollback(release)}
                                className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                                title="Rollback"
                                disabled={isStale}
                            >
                                <ArrowUturnLeftIcon className="w-4 h-4" />
                            </button>
                        )}
                        <button
                            onClick={() => handleUninstall(release)}
                            className="p-1.5 text-red-400 hover:text-red-300 hover:bg-red-900/30 rounded transition-colors"
                            title="Uninstall"
                            disabled={isStale}
                        >
                            <TrashIcon className="w-4 h-4" />
                        </button>
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
