import React, { useState, useRef, useEffect, useCallback } from 'react';
import { BookmarkIcon } from '@heroicons/react/24/solid';
import { PencilSquareIcon, ShareIcon, DocumentTextIcon, CommandLineIcon, FolderIcon } from '@heroicons/react/24/outline';
import { useConfig } from '~/context';
import { getTabIcon } from '~/utils/resourceIcons';

// Map action labels to icons
const actionIconMap: Record<string, any> = {
    'Edit': PencilSquareIcon,
    'Deps': ShareIcon,
    'Logs': DocumentTextIcon,
    'Shell': CommandLineIcon,
    'Files': FolderIcon,
};

interface ContextMenuState {
    x: number;
    y: number;
    tabId: string;
    index: number;
}

interface BottomPanelProps {
    tabs: any[];
    activeTabId: string | null;
    onTabChange: (tabId: string) => void;
    onTabClose: (tabId: string) => void;
    onCloseOthers?: (tabId: string) => void;
    onCloseToRight?: (tabId: string) => void;
    onCloseAll?: () => void;
    onReorder?: (fromIndex: number, toIndex: number) => void;
    onTogglePin?: (tabId: string) => void;
    isTabStale?: (tab: any) => boolean;
    height?: string;
}

export default function BottomPanel({
    tabs,
    activeTabId,
    onTabChange,
    onTabClose,
    onCloseOthers,
    onCloseToRight,
    onCloseAll,
    onReorder,
    onTogglePin,
    isTabStale,
    height = '40%'
}: BottomPanelProps) {
    // Filter out stale tabs unless they are pinned
    const visibleTabs = tabs.filter((tab: any) => !isTabStale?.(tab) || tab.pinned);
    const { getConfig } = useConfig();
    const showTabIcons = getConfig('ui.showTabIcons');

    const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null);
    const [draggedIndex, setDraggedIndex] = useState<number | null>(null);
    const [dropTarget, setDropTarget] = useState<number | null>(null);
    const contextMenuRef = useRef<HTMLDivElement | null>(null);
    const tabRefs = useRef<Record<string, HTMLDivElement | null>>({});
    const scrollPositions = useRef<Record<string, number>>({});
    const contentRefs = useRef<Record<string, HTMLDivElement | null>>({});

    // Track scroll position continuously via capture (works even for non-keepAlive tabs
    // that unmount — we already have the latest position saved before unmount happens)
    const handleScrollCapture = useCallback((tabId: string, e: React.UIEvent) => {
        const target = e.target as HTMLElement;
        if (target.scrollTop > 0) {
            scrollPositions.current[tabId] = target.scrollTop;
        }
    }, []);

    // Restore scroll position when a tab becomes active (content may remount asynchronously)
    useEffect(() => {
        if (!activeTabId) return;
        const saved = scrollPositions.current[activeTabId];
        if (!saved) return;

        const container = contentRefs.current[activeTabId];
        if (!container) return;

        // Retry a few times — non-keepAlive tabs remount and content may render asynchronously
        let attempts = 0;
        const tryRestore = () => {
            const scrollables = container.querySelectorAll<HTMLElement>('*');
            for (const el of scrollables) {
                if (el.scrollHeight > el.clientHeight + 1) {
                    el.scrollTop = saved;
                    if (el.scrollTop > 0) return; // success
                }
            }
            if (++attempts < 5) requestAnimationFrame(tryRestore);
        };
        requestAnimationFrame(tryRestore);
    }, [activeTabId]);

    // Scroll active tab header into view when it changes
    useEffect(() => {
        if (activeTabId && tabRefs.current[activeTabId]) {
            tabRefs.current[activeTabId].scrollIntoView({
                behavior: 'smooth',
                block: 'nearest',
                inline: 'nearest'
            });
        }
    }, [activeTabId]);

    // Close context menu on click outside
    useEffect(() => {
        const handleClickOutside = (e: MouseEvent) => {
            if (contextMenuRef.current && !contextMenuRef.current.contains(e.target as Node)) {
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
        const handleEscape = (e: KeyboardEvent) => {
            if (e.key === 'Escape') setContextMenu(null);
        };
        if (contextMenu) {
            document.addEventListener('keydown', handleEscape);
            return () => document.removeEventListener('keydown', handleEscape);
        }
    }, [contextMenu]);

    const handleContextMenu = (e: React.MouseEvent, tabId: string, index: number) => {
        e.preventDefault();
        setContextMenu({
            x: e.clientX,
            y: e.clientY,
            tabId,
            index
        });
    };

    const handleDragStart = (e: React.DragEvent, index: number) => {
        setDraggedIndex(index);
        e.dataTransfer.effectAllowed = 'move';
        e.dataTransfer.setData('text/plain', index.toString());
        // Add slight delay to allow drag image to form
        requestAnimationFrame(() => {
            (e.target as HTMLElement).style.opacity = '0.5';
        });
    };

    const handleDragEnd = (e: React.DragEvent) => {
        (e.target as HTMLElement).style.opacity = '1';
        setDraggedIndex(null);
        setDropTarget(null);
    };

    const handleDragOver = (e: React.DragEvent, index: number) => {
        e.preventDefault();
        e.dataTransfer.dropEffect = 'move';
        if (draggedIndex !== null && draggedIndex !== index) {
            setDropTarget(index);
        }
    };

    const handleDragLeave = () => {
        setDropTarget(null);
    };

    const handleDrop = (e: React.DragEvent, toIndex: number) => {
        e.preventDefault();
        if (draggedIndex !== null && draggedIndex !== toIndex && onReorder) {
            // Prevent dragging between pinned and unpinned sections
            const draggedTab = tabs[draggedIndex];
            const targetTab = tabs[toIndex];
            if (draggedTab?.pinned === targetTab?.pinned) {
                onReorder(draggedIndex, toIndex);
            }
        }
        setDraggedIndex(null);
        setDropTarget(null);
    };

    const menuAction = (action: string) => {
        if (!contextMenu) return;
        const { tabId, index } = contextMenu;

        switch (action) {
            case 'pin':
                onTogglePin?.(tabId);
                break;
            case 'close':
                onTabClose(tabId);
                break;
            case 'closeOthers':
                onCloseOthers?.(tabId);
                break;
            case 'closeRight':
                onCloseToRight?.(tabId);
                break;
            case 'closeAll':
                onCloseAll?.();
                break;
        }
        setContextMenu(null);
    };

    if (!visibleTabs || visibleTabs.length === 0) return null;

    const isLastTab = !!(contextMenu && contextMenu.index === visibleTabs.length - 1);
    const isOnlyTab = visibleTabs.length === 1;
    const contextMenuTab = contextMenu ? visibleTabs.find((t: any) => t.id === contextMenu.tabId) : null;

    return (
        <div
            className="border-t border-border bg-surface flex flex-col"
            style={{ height: height }}
        >
            <div className="flex items-center bg-background border-b border-border overflow-x-auto">
                {visibleTabs.map((tab: any, index: number) => {
                    const stale = isTabStale?.(tab);
                    return (
                        <div
                            key={tab.id}
                            ref={(el) => { tabRefs.current[tab.id] = el; }}
                            draggable
                            onDragStart={(e) => handleDragStart(e, index)}
                            onDragEnd={handleDragEnd}
                            onDragOver={(e) => handleDragOver(e, index)}
                            onDragLeave={handleDragLeave}
                            onDrop={(e) => handleDrop(e, index)}
                            onContextMenu={(e) => handleContextMenu(e, tab.id, index)}
                            onAuxClick={(e) => {
                                // Middle-click to close tab (button 1 is middle mouse button)
                                if (e.button === 1 && !tab.pinned) {
                                    e.preventDefault();
                                    onTabClose(tab.id);
                                }
                            }}
                            className={`
                                flex items-center px-4 py-2 text-xs font-medium cursor-pointer border-r border-border min-w-[150px] max-w-[250px]
                                ${stale
                                    ? 'bg-amber-900/20 text-amber-400/70 border-b-2 border-b-amber-500/50'
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
                            {tab.pinned && (
                                <BookmarkIcon className="h-3 w-3 mr-1.5 text-primary shrink-0" title="Pinned" />
                            )}
                            {stale && (
                                <span className="mr-1.5 text-amber-400" title="Stale - different context">⚠</span>
                            )}
                            {showTabIcons && (() => {
                                const TabIcon = tab.icon || getTabIcon(tab.id);
                                const ActionIcon = tab.actionLabel ? actionIconMap[tab.actionLabel] : null;
                                return (
                                    <>
                                        {TabIcon && <TabIcon className="h-3.5 w-3.5 mr-1 shrink-0 opacity-70" />}
                                        {ActionIcon && <ActionIcon className="h-3 w-3 mr-1.5 shrink-0 opacity-50" />}
                                    </>
                                );
                            })()}
                            <span className={`truncate flex-1 mr-2 select-none ${stale ? 'line-through opacity-70' : ''}`}>
                                {!showTabIcons && tab.actionLabel ? `${tab.actionLabel}: ${tab.title}` : tab.title}
                            </span>
                            {!tab.pinned && (
                                <button
                                    onClick={(e) => { e.stopPropagation(); onTabClose(tab.id); }}
                                    className="text-gray-500 hover:text-red-400"
                                >
                                    ✕
                                </button>
                            )}
                        </div>
                    );
                })}
            </div>
            {/* Render strategy based on tab.keepAlive:
                - keepAlive: true  -> always mounted, use CSS display to hide (Terminal, LogViewer)
                - keepAlive: false -> only mount when active (YamlEditor, DependencyGraph, etc.)
            */}
            <div className="flex-1 overflow-hidden relative">
                {tabs.map((tab) => {
                    const isActive = tab.id === activeTabId;
                    // keepAlive tabs stay mounted, others only render when active
                    if (!tab.keepAlive && !isActive) return null;
                    return (
                        <div
                            key={tab.id}
                            ref={(el) => { contentRefs.current[tab.id] = el; }}
                            className="absolute inset-0 h-full w-full"
                            style={{ display: isActive ? 'block' : 'none' }}
                            data-selectable-region
                            tabIndex={-1}
                            onScrollCapture={(e) => handleScrollCapture(tab.id, e)}
                        >
                            {tab.content}
                        </div>
                    );
                })}
            </div>

            {/* Context Menu */}
            {contextMenu && (
                <div
                    ref={contextMenuRef}
                    className="fixed bg-surface border border-border rounded shadow-lg py-1 z-50 min-w-[160px]"
                    style={{ left: contextMenu.x, top: contextMenu.y }}
                >
                    <button
                        className="w-full px-3 py-1.5 text-left text-xs hover:bg-background text-text flex items-center gap-2"
                        onClick={() => menuAction('pin')}
                    >
                        <BookmarkIcon className="h-3 w-3" />
                        {contextMenuTab?.pinned ? 'Unpin Tab' : 'Pin Tab'}
                    </button>
                    <div className="border-t border-border my-1" />
                    <button
                        className={`w-full px-3 py-1.5 text-left text-xs hover:bg-background ${contextMenuTab?.pinned ? 'text-gray-500 cursor-not-allowed' : 'text-text'}`}
                        onClick={() => !contextMenuTab?.pinned && menuAction('close')}
                        disabled={contextMenuTab?.pinned}
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
