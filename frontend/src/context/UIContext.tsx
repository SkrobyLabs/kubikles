import React, { createContext, useContext, useState, useEffect, useMemo, useCallback, useRef } from 'react';
import { useK8s } from './K8sContext';

// Bottom panel tab interface
interface BottomTab {
    id: string;
    title: string;
    icon?: React.ComponentType;
    content: React.ReactNode;
    pinned?: boolean;
    context?: string | null; // null = context-independent, undefined = use current context, string = specific context
}

// Modal configuration
interface ModalConfig {
    title: string;
    content: React.ReactNode;
    onClose?: () => void;
    width?: string | number;
}

// Resource identifier for comparison source
interface ResourceIdentifier {
    kind: string;
    namespace: string;
    name: string;
    context: string;
}

// Diagnostic view parameters
interface DiagnosticParams {
    initialSource?: ResourceIdentifier;
    initialTarget?: ResourceIdentifier;
    [key: string]: any; // Allow additional params
}

// Pending search result
interface PendingSearchResult {
    search: string | null;
    autoOpen: boolean;
}

// UI Context value interface
interface UIContextValue {
    // View state
    activeView: string;
    setActiveView: React.Dispatch<React.SetStateAction<string>>;

    // Bottom tabs
    bottomTabs: BottomTab[];
    setBottomTabs: React.Dispatch<React.SetStateAction<BottomTab[]>>;
    activeTabId: string | null;
    setActiveTabId: React.Dispatch<React.SetStateAction<string | null>>;

    // Tab operations
    openTab: (tab: BottomTab) => void;
    updateTab: (tabId: string, updates: Partial<BottomTab>) => void;
    closeTab: (tabId: string) => void;
    closeOtherTabs: (tabId: string) => void;
    closeTabsToRight: (tabId: string) => void;
    closeAllTabs: () => void;
    reorderTabs: (fromIndex: number, toIndex: number) => void;
    togglePinTab: (tabId: string) => void;
    isTabStale: (tab: BottomTab) => boolean;

    // Panel state
    panelHeight: number;
    setPanelHeight: React.Dispatch<React.SetStateAction<number>>;

    // Search navigation
    pendingSearch: string | null;
    navigateWithSearch: (view: string, searchTerm: string, autoOpenDetails?: boolean) => void;
    consumePendingSearch: () => PendingSearchResult;

    // Modal state
    modal: ModalConfig | null;
    openModal: (config: ModalConfig) => void;
    closeModal: () => void;

    // Detail tabs per resource type
    getDetailTab: (resourceType: string, defaultTab: string) => string;
    setDetailTab: (resourceType: string, tab: string) => void;

    // Diagnostic views
    diagnosticParams: DiagnosticParams | null;
    openDiagnostic: (view: string, params?: DiagnosticParams) => void;
    consumeDiagnosticParams: () => DiagnosticParams | null;

    // Resource comparison
    comparisonSource: ResourceIdentifier | null;
    setComparisonSource: (kind: string, namespace: string, name: string) => void;
    clearComparisonSource: () => void;
    compareWithSource: (targetKind: string, targetNamespace: string, targetName: string) => void;
}

const UIContext = createContext<UIContextValue | undefined>(undefined);

export const useUI = (): UIContextValue => {
    const context = useContext(UIContext);
    if (!context) {
        throw new Error('useUI must be used within a UIProvider');
    }
    return context;
};

export const UIProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const { currentContext } = useK8s();

    const [activeView, setActiveView] = useState<string>('pods');
    const [bottomTabs, setBottomTabs] = useState<BottomTab[]>([]);
    const [activeTabId, setActiveTabId] = useState<string | null>(null);
    const [panelHeight, setPanelHeight] = useState<number>(40);
    const [pendingSearch, setPendingSearch] = useState<string | null>(null);
    const [modal, setModal] = useState<ModalConfig | null>(null);

    // Diagnostic view params - used when navigating to diagnostic views with pre-filled data
    const [diagnosticParams, setDiagnosticParams] = useState<DiagnosticParams | null>(null);

    // Comparison source for resource diff feature
    // Format: { kind: 'pod', namespace: 'default', name: 'nginx-xyz', context: 'my-cluster' }
    // Persists across context switches for cross-cluster comparison
    const [comparisonSource, setComparisonSourceState] = useState<ResourceIdentifier | null>(null);

    // Track active detail tab per resource type (e.g., { pod: 'metrics', node: 'basic' })
    const [detailTabByResourceType, setDetailTabByResourceType] = useState<Record<string, string>>({});

    // Track last active tab per K8s context for restoration on context switch
    const activeTabByContextRef = useRef<Record<string, string>>({});
    const previousContextRef = useRef<string>(currentContext);

    // Use ref to store pending search for reliable consumption
    // This avoids timing issues with React 18's batching
    const pendingSearchRef = useRef<string | null>(null);
    const pendingAutoOpenRef = useRef<boolean>(false);

    // Navigate to a view with a pre-filled search term
    // If autoOpenDetails is true, the first matching row will be auto-clicked
    const navigateWithSearch = useCallback((view: string, searchTerm: string, autoOpenDetails: boolean = false): void => {
        pendingSearchRef.current = searchTerm;
        pendingAutoOpenRef.current = autoOpenDetails;
        setPendingSearch(searchTerm); // Trigger re-render
        setActiveView(view);
    }, []);

    // Navigate to a diagnostic view with initial params
    const openDiagnostic = useCallback((view: string, params: DiagnosticParams = {}): void => {
        setDiagnosticParams(params);
        setActiveView(view);
    }, []);

    // Consume diagnostic params (called by diagnostic component when it mounts)
    const consumeDiagnosticParams = useCallback((): DiagnosticParams | null => {
        const result = diagnosticParams;
        setDiagnosticParams(null);
        return result;
    }, [diagnosticParams]);

    // Set comparison source for resource diff (includes current context for cross-cluster comparison)
    const setComparisonSource = useCallback((kind: string, namespace: string, name: string): void => {
        setComparisonSourceState({ kind, namespace, name, context: currentContext });
    }, [currentContext]);

    // Clear comparison source
    const clearComparisonSource = useCallback((): void => {
        setComparisonSourceState(null);
    }, []);

    // Compare current resource with the comparison source
    const compareWithSource = useCallback((targetKind: string, targetNamespace: string, targetName: string): void => {
        if (!comparisonSource) {
            return;
        }
        const params: DiagnosticParams = {
            initialSource: {
                kind: comparisonSource.kind,
                namespace: comparisonSource.namespace,
                name: comparisonSource.name,
                context: comparisonSource.context
            },
            initialTarget: {
                kind: targetKind,
                namespace: targetNamespace,
                name: targetName,
                context: currentContext
            }
        };
        openDiagnostic('resource-diff', params);
        // Clear comparison source after use
        setComparisonSourceState(null);
    }, [comparisonSource, currentContext, openDiagnostic]);

    // Consume pending search (called by ResourceList when it mounts/updates)
    const consumePendingSearch = useCallback((): PendingSearchResult => {
        const result = pendingSearchRef.current;
        const autoOpen = pendingAutoOpenRef.current;
        pendingSearchRef.current = null;
        pendingAutoOpenRef.current = false;
        setPendingSearch(null);
        return { search: result, autoOpen };
    }, []);

    // Handle context switches - save/restore active tab per context
    useEffect(() => {
        const prevContext = previousContextRef.current;

        // Skip if context hasn't actually changed
        if (prevContext === currentContext) return;

        // Save current active tab for the previous context
        if (prevContext && activeTabId) {
            activeTabByContextRef.current[prevContext] = activeTabId;
        }

        // Find visible tabs for the new context
        // A tab is visible if: it's not stale (belongs to current context or is context-independent) OR it's pinned
        const visibleTabs = bottomTabs.filter(tab => {
            const isStale = tab.context && tab.context !== currentContext;
            return !isStale || tab.pinned;
        });

        // Try to restore last active tab for this context
        const savedTabId = activeTabByContextRef.current[currentContext];
        const savedTabStillVisible = savedTabId && visibleTabs.some(t => t.id === savedTabId);

        if (savedTabStillVisible) {
            // Restore the last active tab for this context
            setActiveTabId(savedTabId);
        } else if (visibleTabs.length > 0) {
            // Fall back to first visible tab
            setActiveTabId(visibleTabs[0].id);
        } else {
            // No visible tabs
            setActiveTabId(null);
        }

        // Update previous context ref
        previousContextRef.current = currentContext;
    }, [currentContext, bottomTabs, activeTabId]);

    // Persistence for activeView - save globally (not per-context)
    useEffect(() => {
        const saved = localStorage.getItem('kubikles_active_view');
        if (saved) {
            setActiveView(saved);
        }
    }, []);

    useEffect(() => {
        localStorage.setItem('kubikles_active_view', activeView);
    }, [activeView]);

    // Tab Management (all callbacks memoized to prevent re-renders)
    const openTab = useCallback((tab: BottomTab): void => {
        setBottomTabs(prev => {
            const existingIndex = prev.findIndex(t => t.id === tab.id);
            if (existingIndex >= 0) {
                // Update existing tab - preserve original context
                const newTabs = [...prev];
                newTabs[existingIndex] = {
                    ...prev[existingIndex],
                    ...tab,
                    context: prev[existingIndex].context // Keep original context
                };
                return newTabs;
            }
            // Add new tab - use explicit context if provided, otherwise current context
            // (context: null means context-independent, context: undefined means use current)
            const tabContext = 'context' in tab ? tab.context : currentContext;
            return [...prev, { ...tab, context: tabContext }];
        });
        setActiveTabId(tab.id);
    }, [currentContext]);

    const updateTab = useCallback((tabId: string, updates: Partial<BottomTab>): void => {
        setBottomTabs(prev => prev.map(t =>
            t.id === tabId ? { ...t, ...updates } : t
        ));
    }, []);

    const togglePinTab = useCallback((tabId: string): void => {
        setBottomTabs(prev => {
            const newTabs = prev.map(t =>
                t.id === tabId ? { ...t, pinned: !t.pinned } : t
            );
            // Sort: pinned tabs first, then unpinned (preserve order within each group)
            const pinned = newTabs.filter(t => t.pinned);
            const unpinned = newTabs.filter(t => !t.pinned);
            return [...pinned, ...unpinned];
        });
    }, []);

    const closeTab = useCallback((tabId: string): void => {
        setBottomTabs(prev => {
            // Don't close pinned tabs
            const tab = prev.find(t => t.id === tabId);
            if (tab?.pinned) return prev;

            const closingIndex = prev.findIndex(t => t.id === tabId);
            const newTabs = prev.filter(t => t.id !== tabId);

            // Update active tab if we're closing the active one
            setActiveTabId(currentActive => {
                if (currentActive !== tabId) return currentActive;
                if (newTabs.length === 0) return null;
                // Try to select the tab to the left, otherwise the one that took its place
                const newIndex = Math.min(closingIndex - 1, newTabs.length - 1);
                return newTabs[Math.max(0, newIndex)]?.id || null;
            });

            return newTabs;
        });
    }, []);

    const closeOtherTabs = useCallback((tabId: string): void => {
        setBottomTabs(prev => prev.filter(t => t.id === tabId || t.pinned));
        setActiveTabId(tabId);
    }, []);

    const closeTabsToRight = useCallback((tabId: string): void => {
        setBottomTabs(prev => {
            const index = prev.findIndex(t => t.id === tabId);
            if (index === -1) return prev;
            // Keep tabs to the left (including current) + any pinned tabs to the right
            const leftTabs = prev.slice(0, index + 1);
            const rightPinned = prev.slice(index + 1).filter(t => t.pinned);
            return [...leftTabs, ...rightPinned];
        });
    }, []);

    const closeAllTabs = useCallback((): void => {
        setBottomTabs(prev => prev.filter(t => t.pinned));
        setActiveTabId(prev => prev);
    }, []);

    const openModal = useCallback((config: ModalConfig): void => {
        setModal(config);
    }, []);

    const closeModal = useCallback((): void => {
        setModal(null);
    }, []);

    // Get the active detail tab for a resource type, with fallback to default
    const getDetailTab = useCallback((resourceType: string, defaultTab: string): string => {
        return detailTabByResourceType[resourceType] || defaultTab;
    }, [detailTabByResourceType]);

    // Set the active detail tab for a resource type
    const setDetailTab = useCallback((resourceType: string, tab: string): void => {
        setDetailTabByResourceType(prev => ({
            ...prev,
            [resourceType]: tab
        }));
    }, []);

    const reorderTabs = useCallback((fromIndex: number, toIndex: number): void => {
        setBottomTabs(prev => {
            const newTabs = [...prev];
            const [removed] = newTabs.splice(fromIndex, 1);
            newTabs.splice(toIndex, 0, removed);
            return newTabs;
        });
    }, []);

    // Check if a tab is stale (belongs to a different context)
    const isTabStale = useCallback((tab: BottomTab): boolean => {
        return tab.context !== null && tab.context !== undefined && tab.context !== currentContext;
    }, [currentContext]);

    // Memoize context value to prevent unnecessary re-renders of consumers
    // Note: activeMenuId moved to MenuContext for better performance
    const value: UIContextValue = useMemo(() => ({
        activeView,
        setActiveView,
        bottomTabs,
        setBottomTabs,
        activeTabId,
        setActiveTabId,
        openTab,
        updateTab,
        closeTab,
        closeOtherTabs,
        closeTabsToRight,
        closeAllTabs,
        reorderTabs,
        togglePinTab,
        isTabStale,
        panelHeight,
        setPanelHeight,
        pendingSearch,
        navigateWithSearch,
        consumePendingSearch,
        modal,
        openModal,
        closeModal,
        getDetailTab,
        setDetailTab,
        diagnosticParams,
        openDiagnostic,
        consumeDiagnosticParams,
        comparisonSource,
        setComparisonSource,
        clearComparisonSource,
        compareWithSource
    }), [
        activeView,
        bottomTabs,
        activeTabId,
        openTab,
        updateTab,
        closeTab,
        closeOtherTabs,
        closeTabsToRight,
        closeAllTabs,
        reorderTabs,
        togglePinTab,
        isTabStale,
        panelHeight,
        pendingSearch,
        navigateWithSearch,
        consumePendingSearch,
        modal,
        openModal,
        closeModal,
        getDetailTab,
        setDetailTab,
        diagnosticParams,
        openDiagnostic,
        consumeDiagnosticParams,
        comparisonSource,
        setComparisonSource,
        clearComparisonSource,
        compareWithSource
    ]);

    return (
        <UIContext.Provider value={value}>
            {children}
        </UIContext.Provider>
    );
};
