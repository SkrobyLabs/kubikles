import React, { createContext, useContext, useState, useCallback, useEffect, useMemo } from 'react';
import { SetRequestCancellationEnabled, SetForceHTTP1, SetClientPoolSize } from 'wailsjs/go/main/App';
import { validateConfig } from '~/lib/validation';
import type { SidebarLayoutSection } from '~/constants/menuStructure';

interface LogSearchConfig {
    debounceMs: number;
    searchOnEnter: boolean;
    useRegex: boolean;
    filterOnly: boolean;
    contextLinesBefore: number;
    contextLinesAfter: number;
}

interface LogsConfig {
    lineWrap: boolean;
    showTimestamps: boolean;
    position: 'start' | 'end';
    search: LogSearchConfig;
}

interface AIConfig {
    model: string;
    panelWidth: number;
    allowedTools: string[];
}

interface UIConfig {
    searchDebounceMs: number;
    copyFeedbackMs: number;
    scrollZoomEnabled: boolean;
    showTabIcons: boolean;
    sidebar?: {
        layout?: SidebarLayoutSection[];
        excludedItems?: string[];
    };
}

interface KubernetesConfig {
    apiTimeoutMs: number;
    metricsPollIntervalMs: number;
    connectionTestTimeoutSeconds: number;
    nodeDebugImage: string;
}

interface MetricsConfig {
    preferredSource: 'auto' | 'k8s' | 'prometheus';
}

interface PerformanceConfig {
    pollIntervalMs: number;
    eventCoalescerMs: number;
    enableRequestCancellation: boolean;
    forceHttp1: boolean;
    clientPoolSize: number;
}

interface DebugConfig {
    showDebugIcon: boolean;
    showLogSourceMarkers: boolean;
}

interface AppConfig {
    logs: LogsConfig;
    ai: AIConfig;
    ui: UIConfig;
    kubernetes: KubernetesConfig;
    metrics: MetricsConfig;
    performance: PerformanceConfig;
    debug: DebugConfig;
}

interface ConfigContextValue {
    config: AppConfig;
    getConfig: (path: string) => any;
    setConfig: (path: string, value: any) => void;
    updateConfig: (newConfig: AppConfig) => void;
    resetConfig: () => void;
    getConfigJson: () => string;
    defaultConfig: AppConfig;
    showConfigEditor: boolean;
    openConfigEditor: () => void;
    closeConfigEditor: () => void;
}

const ConfigContext = createContext<ConfigContextValue | undefined>(undefined);

// Default configuration
const defaultConfig: AppConfig = {
    logs: {
        lineWrap: true,
        showTimestamps: false,
        position: 'end',
        search: {
            debounceMs: 200,
            searchOnEnter: true,
            useRegex: false,
            filterOnly: true,
            contextLinesBefore: 1,
            contextLinesAfter: 5
        }
    },
    ai: {
        model: 'sonnet',
        panelWidth: 384,
        allowedTools: [
            'get_pod_logs', 'get_resource_yaml', 'list_resources',
            'get_events', 'describe_resource', 'list_crds',
            'list_custom_resources', 'get_custom_resource_yaml'
        ]
    },
    ui: {
        // Debounce delay for resource list search (ms)
        searchDebounceMs: 150,
        // How long "Copied!" feedback shows (ms)
        copyFeedbackMs: 2000,
        // Enable Cmd/Ctrl+Scroll to zoom in/out
        scrollZoomEnabled: false,
        // Display resource type icons in tab titles
        showTabIcons: true
    },
    kubernetes: {
        // API request timeout (ms). Increase for slow clusters.
        apiTimeoutMs: 60000,
        // Poll interval for Kubernetes CPU/Memory metrics (ms)
        metricsPollIntervalMs: 30000,
        // Connection test timeout (seconds). When switching contexts, a quick
        // connectivity check is performed to fail fast if the cluster is unreachable.
        // Increase if you have high-latency clusters that need more time.
        connectionTestTimeoutSeconds: 5,
        // Default container image for node debug shell sessions
        nodeDebugImage: 'alpine:latest'
    },
    metrics: {
        // Preferred metrics source: "auto" (try K8s first, fallback to Prometheus),
        // "k8s" (K8s Metrics API only), or "prometheus" (Prometheus only)
        preferredSource: "auto"
    },
    performance: {
        // Poll interval for performance panel (ms)
        pollIntervalMs: 1500,
        // Frame interval for resource event batching (ms). Lower = more responsive, higher = less CPU.
        eventCoalescerMs: 16,
        // Enable actual HTTP request cancellation when navigating between views.
        // Due to a Go HTTP/2 bug (golang/go#34944), cancelling requests can cause
        // performance issues. When disabled, requests complete in background
        // but stale results are ignored. Disable if experiencing slow navigation.
        enableRequestCancellation: true,
        // Force HTTP/1.1 instead of HTTP/2. HTTP/1.1 opens multiple TCP connections
        // for parallel requests, avoiding HTTP/2 flow control bottlenecks.
        // Requires context switch to take effect.
        forceHttp1: false,
        // Additional K8s client connections for better parallelism.
        // 0 = just main connection. Requires context switch.
        clientPoolSize: 0
    },
    debug: {
        // Show debug download button in log viewer. Downloads logs with source
        // markers indicating how each line was fetched: [INITIAL], [STREAM],
        // [BEFORE], [AFTER]. Useful for debugging log viewer pagination issues.
        showDebugIcon: false,
        showLogSourceMarkers: false
    }
};

// Migrate old config structure to new
const migrateConfig = (config: any): AppConfig => {
    const migrated: any = { ...config };

    // Migrate metrics.pollIntervalMs -> kubernetes.metricsPollIntervalMs
    if (config.metrics?.pollIntervalMs !== undefined) {
        if (!migrated.kubernetes) migrated.kubernetes = {};
        if (migrated.kubernetes.metricsPollIntervalMs === undefined) {
            migrated.kubernetes.metricsPollIntervalMs = config.metrics.pollIntervalMs;
        }
        delete migrated.metrics;
    }

    return migrated as AppConfig;
};

// Storage key for localStorage
const CONFIG_STORAGE_KEY = 'kubikles_settings';

// Deep merge helper
const deepMerge = (target: any, source: any): any => {
    const result = { ...target };
    for (const key in source) {
        if (source[key] && typeof source[key] === 'object' && !Array.isArray(source[key])) {
            result[key] = deepMerge(result[key] || {}, source[key]);
        } else {
            result[key] = source[key];
        }
    }
    return result;
};

// Extract only values that differ from defaults (for storage)
// This ensures users get new defaults when we update them, unless they explicitly changed the value
const getDiff = (current: any, defaults: any): any => {
    const diff: any = {};
    for (const key in current) {
        const currentVal = current[key];
        const defaultVal = defaults?.[key];

        if (currentVal && typeof currentVal === 'object' && !Array.isArray(currentVal)) {
            const nestedDiff = getDiff(currentVal, defaultVal || {});
            if (Object.keys(nestedDiff).length > 0) {
                diff[key] = nestedDiff;
            }
        } else if (Array.isArray(currentVal)) {
            // For arrays of primitives (strings), sorted comparison is fine.
            // For arrays of objects, compare by JSON serialization (order matters).
            const hasObjects = currentVal.length > 0 && typeof currentVal[0] === 'object';
            if (hasObjects) {
                if (JSON.stringify(currentVal) !== JSON.stringify(defaultVal || [])) {
                    diff[key] = currentVal;
                }
            } else {
                if (JSON.stringify([...(currentVal)].sort()) !== JSON.stringify([...(defaultVal || [])].sort())) {
                    diff[key] = currentVal;
                }
            }
        } else if (currentVal !== defaultVal) {
            diff[key] = currentVal;
        }
    }
    return diff;
};

// Get nested value by path (e.g., "logs.search.debounceMs")
const getByPath = (obj: any, path: string): any => {
    return path.split('.').reduce((acc: any, part: any) => acc?.[part], obj);
};

// Set nested value by path
const setByPath = (obj: any, path: string, value: any): any => {
    const parts = path.split('.');
    const result = JSON.parse(JSON.stringify(obj)); // Deep clone
    let current = result;
    for (let i = 0; i < parts.length - 1; i++) {
        if (!current[parts[i]]) {
            current[parts[i]] = {};
        }
        current = current[parts[i]];
    }
    current[parts[parts.length - 1]] = value;
    return result;
};

export const useConfig = (): ConfigContextValue => {
    const context = useContext(ConfigContext);
    if (!context) {
        throw new Error('useConfig must be used within a ConfigProvider');
    }
    return context;
};

export const ConfigProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
    // Load config from localStorage on mount
    const [config, setConfigState] = useState<AppConfig>(() => {
        try {
            const saved = localStorage.getItem(CONFIG_STORAGE_KEY);
            if (saved) {
                const parsed = JSON.parse(saved);

                // Validate parsed config with Zod
                const validation = validateConfig(parsed);
                if (!validation.valid) {
                    console.warn('Config validation failed, using defaults for invalid fields:', validation.issues);
                }

                // Migrate old config structure if needed
                const migrated = migrateConfig(parsed);
                // Merge with defaults to handle new config keys
                return deepMerge(defaultConfig, migrated);
            }
        } catch (e: any) {
            console.error('Failed to load config from localStorage:', e);
        }
        return defaultConfig;
    });

    const [showConfigEditor, setShowConfigEditor] = useState<boolean>(false);

    // Persist config to localStorage (only save values that differ from defaults)
    useEffect(() => {
        try {
            const diff = getDiff(config, defaultConfig);
            if (Object.keys(diff).length > 0) {
                localStorage.setItem(CONFIG_STORAGE_KEY, JSON.stringify(diff));
            } else {
                // No differences - remove stored config so defaults are used
                localStorage.removeItem(CONFIG_STORAGE_KEY);
            }
        } catch (e: any) {
            console.error('Failed to save config to localStorage:', e);
        }
    }, [config]);

    // Sync request cancellation setting to backend
    useEffect(() => {
        const enabled = config.performance?.enableRequestCancellation ?? true;
        SetRequestCancellationEnabled(enabled).catch((err: any) => {
            console.error('Failed to set request cancellation setting:', err);
        });
    }, [config.performance?.enableRequestCancellation]);

    // Sync HTTP protocol setting to backend
    useEffect(() => {
        const forceHttp1 = config.performance?.forceHttp1 ?? false;
        SetForceHTTP1(forceHttp1).catch((err: any) => {
            console.error('Failed to set HTTP/1 setting:', err);
        });
    }, [config.performance?.forceHttp1]);

    // Sync client pool size setting to backend
    useEffect(() => {
        const poolSize = config.performance?.clientPoolSize ?? 0;
        SetClientPoolSize(poolSize).catch((err: any) => {
            console.error('Failed to set client pool size:', err);
        });
    }, [config.performance?.clientPoolSize]);

    // Get a config value by path
    const getConfig = useCallback((path: string): any => {
        const value = getByPath(config, path);
        // Fall back to default if not set
        if (value === undefined) {
            return getByPath(defaultConfig, path);
        }
        return value;
    }, [config]);

    // Set a config value by path
    const setConfig = useCallback((path: string, value: any): void => {
        setConfigState(prev => setByPath(prev, path, value));
    }, []);

    // Update entire config (used by config editor)
    const updateConfig = useCallback((newConfig: AppConfig): void => {
        // Merge with defaults to ensure all required fields exist
        setConfigState(deepMerge(defaultConfig, newConfig));
    }, []);

    // Reset config to defaults
    const resetConfig = useCallback((): void => {
        setConfigState(defaultConfig);
    }, []);

    // Get config as JSON string (for editor)
    const getConfigJson = useCallback((): string => {
        return JSON.stringify(config, null, 2);
    }, [config]);

    // Open/close config editor
    const openConfigEditor = useCallback((): void => {
        setShowConfigEditor(true);
    }, []);

    const closeConfigEditor = useCallback((): void => {
        setShowConfigEditor(false);
    }, []);

    const value: ConfigContextValue = useMemo(() => ({
        config,
        getConfig,
        setConfig,
        updateConfig,
        resetConfig,
        getConfigJson,
        defaultConfig,
        showConfigEditor,
        openConfigEditor,
        closeConfigEditor
    }), [config, getConfig, setConfig, updateConfig, resetConfig, getConfigJson, showConfigEditor, openConfigEditor, closeConfigEditor]);

    return (
        <ConfigContext.Provider value={value}>
            {children}
        </ConfigContext.Provider>
    );
};
