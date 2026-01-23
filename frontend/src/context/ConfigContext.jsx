import React, { createContext, useContext, useState, useCallback, useEffect, useMemo } from 'react';

const ConfigContext = createContext();

// Default configuration
const defaultConfig = {
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
    portForwards: {
        // Auto-start mode on app launch:
        // - "all": Start all port forwards that were running when app was closed
        // - "favorites": Only start favorites that were running when app was closed
        // - "none": Don't auto-start any port forwards
        autoStartMode: "favorites"
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
    metrics: {
        // Poll interval for node/pod metrics (ms)
        pollIntervalMs: 30000
    },
    performance: {
        // Poll interval for performance panel (ms)
        pollIntervalMs: 1500,
        // Frame interval for resource event batching (ms). Lower = more responsive, higher = less CPU.
        eventCoalescerMs: 16
    }
};

// Storage key for localStorage
const CONFIG_STORAGE_KEY = 'kubikles_settings';

// Deep merge helper
const deepMerge = (target, source) => {
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
const getDiff = (current, defaults) => {
    const diff = {};
    for (const key in current) {
        const currentVal = current[key];
        const defaultVal = defaults?.[key];

        if (currentVal && typeof currentVal === 'object' && !Array.isArray(currentVal)) {
            const nestedDiff = getDiff(currentVal, defaultVal || {});
            if (Object.keys(nestedDiff).length > 0) {
                diff[key] = nestedDiff;
            }
        } else if (currentVal !== defaultVal) {
            diff[key] = currentVal;
        }
    }
    return diff;
};

// Get nested value by path (e.g., "logs.search.debounceMs")
const getByPath = (obj, path) => {
    return path.split('.').reduce((acc, part) => acc?.[part], obj);
};

// Set nested value by path
const setByPath = (obj, path, value) => {
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

export const useConfig = () => {
    const context = useContext(ConfigContext);
    if (!context) {
        throw new Error('useConfig must be used within a ConfigProvider');
    }
    return context;
};

export const ConfigProvider = ({ children }) => {
    // Load config from localStorage on mount
    const [config, setConfigState] = useState(() => {
        try {
            const saved = localStorage.getItem(CONFIG_STORAGE_KEY);
            if (saved) {
                const parsed = JSON.parse(saved);
                // Merge with defaults to handle new config keys
                return deepMerge(defaultConfig, parsed);
            }
        } catch (e) {
            console.error('Failed to load config from localStorage:', e);
        }
        return defaultConfig;
    });

    const [showConfigEditor, setShowConfigEditor] = useState(false);

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
        } catch (e) {
            console.error('Failed to save config to localStorage:', e);
        }
    }, [config]);

    // Get a config value by path
    const getConfig = useCallback((path) => {
        const value = getByPath(config, path);
        // Fall back to default if not set
        if (value === undefined) {
            return getByPath(defaultConfig, path);
        }
        return value;
    }, [config]);

    // Set a config value by path
    const setConfig = useCallback((path, value) => {
        setConfigState(prev => setByPath(prev, path, value));
    }, []);

    // Update entire config (used by config editor)
    const updateConfig = useCallback((newConfig) => {
        // Merge with defaults to ensure all required fields exist
        setConfigState(deepMerge(defaultConfig, newConfig));
    }, []);

    // Reset config to defaults
    const resetConfig = useCallback(() => {
        setConfigState(defaultConfig);
    }, []);

    // Get config as JSON string (for editor)
    const getConfigJson = useCallback(() => {
        return JSON.stringify(config, null, 2);
    }, [config]);

    // Open/close config editor
    const openConfigEditor = useCallback(() => {
        setShowConfigEditor(true);
    }, []);

    const closeConfigEditor = useCallback(() => {
        setShowConfigEditor(false);
    }, []);

    const value = useMemo(() => ({
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
