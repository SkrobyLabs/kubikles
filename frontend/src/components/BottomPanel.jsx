import React from 'react';

export default function BottomPanel({ tabs, activeTabId, onTabChange, onTabClose, height = '40%' }) {
    const activeTab = tabs.find(t => t.id === activeTabId);

    if (!tabs || tabs.length === 0) return null;

    return (
        <div
            className="border-t border-border bg-surface flex flex-col"
            style={{ height: height }}
        >
            <div className="flex items-center bg-background border-b border-border overflow-x-auto">
                {tabs.map(tab => (
                    <div
                        key={tab.id}
                        className={`
                            flex items-center px-4 py-2 text-xs font-medium cursor-pointer border-r border-border min-w-[150px] max-w-[250px]
                            ${tab.id === activeTabId ? 'bg-surface text-primary border-b-2 border-b-primary' : 'text-gray-400 hover:text-text hover:bg-surface'}
                        `}
                        onClick={() => onTabChange(tab.id)}
                    >
                        <span className="truncate flex-1 mr-2">{tab.title}</span>
                        <button
                            onClick={(e) => { e.stopPropagation(); onTabClose(tab.id); }}
                            className="text-gray-500 hover:text-red-400"
                        >
                            ✕
                        </button>
                    </div>
                ))}
            </div>
            <div className="flex-1 overflow-hidden relative">
                {tabs.map(tab => (
                    <div
                        key={tab.id}
                        className="h-full w-full"
                        style={{ display: tab.id === activeTabId ? 'block' : 'none' }}
                    >
                        {tab.content}
                    </div>
                ))}
            </div>
        </div>
    );
}
