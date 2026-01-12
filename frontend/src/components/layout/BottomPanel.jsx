import React, { useState, useRef, useEffect } from 'react';

export default function BottomPanel({
    tabs,
    activeTabId,
    onTabChange,
    onTabClose,
    onCloseOthers,
    onCloseToRight,
    onCloseAll,
    onCloseStaleTabs,
    onReorder,
    isTabStale,
    height = '40%'
}) {
    const activeTab = tabs.find(t => t.id === activeTabId);
    const [contextMenu, setContextMenu] = useState(null);
    const [draggedIndex, setDraggedIndex] = useState(null);
    const [dropTarget, setDropTarget] = useState(null);
    const contextMenuRef = useRef(null);

    // Close context menu on click outside
    useEffect(() => {
        const handleClickOutside = (e) => {
            if (contextMenuRef.current && !contextMenuRef.current.contains(e.target)) {
                setContextMenu(null);
            }
        };
        if (contextMenu) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [contextMenu]);

    // Close context menu on escape
    useEffect(() => {
        const handleEscape = (e) => {
            if (e.key === 'Escape') setContextMenu(null);
        };
        if (contextMenu) {
            document.addEventListener('keydown', handleEscape);
            return () => document.removeEventListener('keydown', handleEscape);
        }
    }, [contextMenu]);

    const handleContextMenu = (e, tabId, index) => {
        e.preventDefault();
        setContextMenu({
            x: e.clientX,
            y: e.clientY,
            tabId,
            index
        });
    };

    const handleDragStart = (e, index) => {
        setDraggedIndex(index);
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', index.toString());
        // Add slight delay to allow drag image to form
        requestAnimationFrame(() => {
            e.target.style.opacity = '0.5';
        });
    };

    const handleDragEnd = (e) => {
        e.target.style.opacity = '1';
        setDraggedIndex(null);
        setDropTarget(null);
    };

    const handleDragOver = (e, index) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        if (draggedIndex !== null && draggedIndex !== index) {
            setDropTarget(index);
        }
    };

    const handleDragLeave = () => {
        setDropTarget(null);
    };

    const handleDrop = (e, toIndex) => {
        e.preventDefault();
        if (draggedIndex !== null && draggedIndex !== toIndex && onReorder) {
            onReorder(draggedIndex, toIndex);
        }
        setDraggedIndex(null);
        setDropTarget(null);
    };

    const menuAction = (action) => {
        if (!contextMenu) return;
        const { tabId, index } = contextMenu;

        switch (action) {
            case 'close':
                onTabClose(tabId);
                break;
            case 'closeOthers':
                onCloseOthers?.(tabId);
                break;
            case 'closeRight':
                onCloseToRight?.(tabId);
                break;
            case 'closeStaleTabs':
                onCloseStaleTabs?.();
                break;
            case 'closeAll':
                onCloseAll?.();
                break;
        }
        setContextMenu(null);
    };

    if (!tabs || tabs.length === 0) return null;

    const isLastTab = contextMenu && contextMenu.index === tabs.length - 1;
    const isOnlyTab = tabs.length === 1;
    const hasStaleTabs = tabs.some(t => isTabStale?.(t));

    return (
        <div
            className="border-t border-border bg-surface flex flex-col"
            style={{ height: height }}
        >
            <div className="flex items-center bg-background border-b border-border overflow-x-auto">
                {tabs.map((tab, index) => {
                    const stale = isTabStale?.(tab);
                    return (
                        <div
                            key={tab.id}
                            draggable
                            onDragStart={(e) => handleDragStart(e, index)}
                            onDragEnd={handleDragEnd}
                            onDragOver={(e) => handleDragOver(e, index)}
                            onDragLeave={handleDragLeave}
                            onDrop={(e) => handleDrop(e, index)}
                            onContextMenu={(e) => handleContextMenu(e, tab.id, index)}
                            className={`
                                flex items-center px-4 py-2 text-xs font-medium cursor-pointer border-r border-border min-w-[150px] max-w-[250px]
                                ${stale
                                    ? 'bg-red-900/20 text-red-400/70 border-b-2 border-b-red-500/50'
                                    : tab.id === activeTabId
                                        ? 'bg-surface text-primary border-b-2 border-b-primary'
                                        : 'text-gray-400 hover:text-text hover:bg-surface'
                                }
                                ${dropTarget === index ? 'border-l-2 border-l-primary' : ''}
                                ${draggedIndex === index ? 'opacity-50' : ''}
                                transition-colors
                            `}
                            onClick={() => onTabChange(tab.id)}
                            title={stale ? `From context: ${tab.context} (read-only)` : undefined}
                        >
                            {stale && (
                                <span className="mr-1.5 text-red-400" title="Stale - different context">⚠</span>
                            )}
                            <span className={`truncate flex-1 mr-2 select-none ${stale ? 'line-through opacity-70' : ''}`}>
                                {tab.title}
                            </span>
                            <button
                                onClick={(e) => { e.stopPropagation(); onTabClose(tab.id); }}
                                className="text-gray-500 hover:text-red-400"
                            >
                                ✕
                            </button>
                        </div>
                    );
                })}
            </div>
            <div className="flex-1 overflow-hidden relative">
                {activeTab && (
                    <div className="h-full w-full">
                        {activeTab.content}
                    </div>
                )}
            </div>

            {/* Context Menu */}
            {contextMenu && (
                <div
                    ref={contextMenuRef}
                    className="fixed bg-surface border border-border rounded shadow-lg py-1 z-50 min-w-[160px]"
                    style={{ left: contextMenu.x, top: contextMenu.y }}
                >
                    <button
                        className="w-full px-3 py-1.5 text-left text-xs hover:bg-background text-text"
                        onClick={() => menuAction('close')}
                    >
                        Close
                    </button>
                    <button
                        className={`w-full px-3 py-1.5 text-left text-xs hover:bg-background ${isOnlyTab ? 'text-gray-500 cursor-not-allowed' : 'text-text'}`}
                        onClick={() => !isOnlyTab && menuAction('closeOthers')}
                        disabled={isOnlyTab}
                    >
                        Close Other Tabs
                    </button>
                    <button
                        className={`w-full px-3 py-1.5 text-left text-xs hover:bg-background ${isLastTab ? 'text-gray-500 cursor-not-allowed' : 'text-text'}`}
                        onClick={() => !isLastTab && menuAction('closeRight')}
                        disabled={isLastTab}
                    >
                        Close Tabs to Right
                    </button>
                    {hasStaleTabs && (
                        <>
                            <div className="border-t border-border my-1" />
                            <button
                                className="w-full px-3 py-1.5 text-left text-xs hover:bg-background text-red-400"
                                onClick={() => menuAction('closeStaleTabs')}
                            >
                                Close Stale Tabs
                            </button>
                        </>
                    )}
                    <div className="border-t border-border my-1" />
                    <button
                        className="w-full px-3 py-1.5 text-left text-xs hover:bg-background text-text"
                        onClick={() => menuAction('closeAll')}
                    >
                        Close All Tabs
                    </button>
                </div>
            )}
        </div>
    );
}
