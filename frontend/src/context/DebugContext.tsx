import React, { createContext, useContext, useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { SetDebugEnabled } from 'wailsjs/go/main/App';

const DEBUG_STORAGE_KEY = 'kubikles-debug-enabled';
const MAX_LOGS = 1000;

// Log sources
export const DEBUG_SOURCE = {
    FRONTEND: 'fe',
    BACKEND: 'be',
} as const;

export type DebugSource = typeof DEBUG_SOURCE[keyof typeof DEBUG_SOURCE];

// Log categories
export const DEBUG_CATEGORIES = {
    K8S: 'k8s',
    WATCHER: 'watcher',
    HELM: 'helm',
    PORTFORWARD: 'portforward',
    TERMINAL: 'terminal',
    AI: 'ai',
    CONFIG: 'config',
    UI: 'ui',
    WAILS: 'wails',
    PERFORMANCE: 'performance',
} as const;

export type DebugCategory = typeof DEBUG_CATEGORIES[keyof typeof DEBUG_CATEGORIES];

interface CategoryColor {
    console: string;
    ui: string;
}

export const CATEGORY_COLORS: Record<DebugCategory, CategoryColor> = {
    [DEBUG_CATEGORIES.K8S]: { console: '#60A5FA', ui: 'text-blue-400' },
    [DEBUG_CATEGORIES.WATCHER]: { console: '#4CC38A', ui: 'text-emerald-400' },
    [DEBUG_CATEGORIES.HELM]: { console: '#818CF8', ui: 'text-indigo-400' },
    [DEBUG_CATEGORIES.PORTFORWARD]: { console: '#F472B6', ui: 'text-pink-300' },
    [DEBUG_CATEGORIES.TERMINAL]: { console: '#A78BFA', ui: 'text-purple-400' },
    [DEBUG_CATEGORIES.AI]: { console: '#F59E0B', ui: 'text-amber-400' },
    [DEBUG_CATEGORIES.CONFIG]: { console: '#14B8A6', ui: 'text-teal-400' },
    [DEBUG_CATEGORIES.UI]: { console: '#EC4899', ui: 'text-pink-400' },
    [DEBUG_CATEGORIES.WAILS]: { console: '#6366F1', ui: 'text-indigo-500' },
    [DEBUG_CATEGORIES.PERFORMANCE]: { console: '#F97316', ui: 'text-orange-400' },
};

export interface DebugLogEntry {
    id: number;
    timestamp: string;
    source: DebugSource;
    category: DebugCategory;
    message: string;
    details: unknown | null;
}

interface DebugContextValue {
    isDebugMode: boolean;
    toggleDebugMode: () => void;
    enableDebugMode: () => void;
    disableDebugMode: () => void;
    logs: DebugLogEntry[];
    clearLogs: () => void;
}

const DebugContext = createContext<DebugContextValue | undefined>(undefined);

export const useDebug = (): DebugContextValue => {
    const context = useContext(DebugContext);
    if (!context) {
        throw new Error('useDebug must be used within a DebugProvider');
    }
    return context;
};

interface UseDebugLogReturn {
    log: (message: string, details?: unknown | null) => void;
    isDebugEnabled: boolean;
}

export function useDebugLog(category: DebugCategory): UseDebugLogReturn {
    const { isDebugMode } = useDebug();
    // This hook is a convenience for components - logs go through the FE event system
    const log = useCallback((message: string, details: unknown | null = null): void => {
        if (!isDebugMode) return;
        window.dispatchEvent(new CustomEvent('frontend-debug-log', {
            detail: { category, message, details }
        }));
    }, [isDebugMode, category]);
    return { log, isDebugEnabled: isDebugMode };
}

export const DebugProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    const [isDebugMode, setIsDebugMode] = useState<boolean>(() => {
        try {
            return localStorage.getItem(DEBUG_STORAGE_KEY) === 'true';
        } catch {
            return false;
        }
    });

    const [logs, setLogs] = useState<DebugLogEntry[]>([]);
    const isDebugModeRef = useRef(isDebugMode);

    // Keep ref in sync
    useEffect(() => {
        isDebugModeRef.current = isDebugMode;
    }, [isDebugMode]);

    // Persist and sync with backend
    useEffect(() => {
        try {
            localStorage.setItem(DEBUG_STORAGE_KEY, isDebugMode.toString());
        } catch { /* ignore */ }
        SetDebugEnabled(isDebugMode).catch(() => { /* backend may not be ready */ });
    }, [isDebugMode]);

    const addLogEntry = useCallback((source: DebugSource, category: DebugCategory, message: string, details: unknown | null = null): void => {
        const timestamp = new Date().toISOString();
        const logEntry: DebugLogEntry = {
            id: Date.now() + Math.random(),
            timestamp,
            source,
            category,
            message,
            details,
        };

        // Console output with styling
        const color = CATEGORY_COLORS[category]?.console || '#9CA3AF';
        const sourceLabel = source === DEBUG_SOURCE.BACKEND ? 'BE' : 'FE';

        if (details !== null && details !== undefined) {
            console.groupCollapsed(
                `%c[${sourceLabel}]%c[${category.toUpperCase()}]%c ${message}`,
                'color: #6B7280; font-weight: bold',
                `color: ${color}; font-weight: bold`,
                'color: inherit'
            );
            console.log(details);
            console.groupEnd();
        } else {
            console.log(
                `%c[${sourceLabel}]%c[${category.toUpperCase()}]%c ${message}`,
                'color: #6B7280; font-weight: bold',
                `color: ${color}; font-weight: bold`,
                'color: inherit'
            );
        }

        setLogs(prev => {
            const newLogs = [logEntry, ...prev];
            return newLogs.length > MAX_LOGS ? newLogs.slice(0, MAX_LOGS) : newLogs;
        });
    }, []);

    // Listen for backend debug events (Wails "debug:log")
    useEffect(() => {
        const handler = (category: string, message: string, details?: unknown): void => {
            if (!isDebugModeRef.current) return;
            addLogEntry(DEBUG_SOURCE.BACKEND, category as DebugCategory, message, details || null);
        };
        const cancel = EventsOn('debug:log', handler);
        return () => { cancel(); };
    }, [addLogEntry]);

    // Listen for frontend debug events (CustomEvent "frontend-debug-log")
    useEffect(() => {
        const handler = (e: Event): void => {
            if (!isDebugModeRef.current) return;
            const { category, message, details } = (e as CustomEvent).detail;
            addLogEntry(DEBUG_SOURCE.FRONTEND, category as DebugCategory, message, details || null);
        };
        window.addEventListener('frontend-debug-log', handler);
        return () => { window.removeEventListener('frontend-debug-log', handler); };
    }, [addLogEntry]);

    const toggleDebugMode = useCallback((): void => {
        setIsDebugMode(prev => !prev);
    }, []);

    const enableDebugMode = useCallback((): void => {
        setIsDebugMode(true);
    }, []);

    const disableDebugMode = useCallback((): void => {
        setIsDebugMode(false);
    }, []);

    const clearLogs = useCallback((): void => {
        setLogs([]);
    }, []);

    const value: DebugContextValue = useMemo(() => ({
        isDebugMode,
        toggleDebugMode,
        enableDebugMode,
        disableDebugMode,
        logs,
        clearLogs,
    }), [isDebugMode, toggleDebugMode, enableDebugMode, disableDebugMode, logs, clearLogs]);

    return (
        <DebugContext.Provider value={value}>
            {children}
        </DebugContext.Provider>
    );
};
