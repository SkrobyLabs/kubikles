import { useState, useEffect, useCallback, useRef } from 'react';
import {
    GetPortForwardConfigs,
    GetActivePortForwards,
    AddPortForwardConfig,
    UpdatePortForwardConfig,
    DeletePortForwardConfig,
    StartPortForward,
    StopPortForward,
    GetAvailablePort
} from '../../wailsjs/go/main/App';

// Global event listener - only one instance to avoid Wails EventsOff issues
let globalEventHandler = null;
const subscribers = new Set();

/**
 * Hook for managing port forwards.
 * Port forwards persist across context switches and app restarts.
 *
 * @param {string} contextFilter - Optional context to filter configs by
 * @param {boolean} isVisible - Whether the component is visible
 */
export const usePortForwards = (contextFilter = '', isVisible = true) => {
    const [configs, setConfigs] = useState([]);
    const [activeForwards, setActiveForwards] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    // Fetch configs and active forwards
    const fetchData = useCallback(async () => {
        if (!isVisible) return;

        setLoading(true);
        try {
            const [cfgs, active] = await Promise.all([
                GetPortForwardConfigs(contextFilter),
                GetActivePortForwards()
            ]);
            setConfigs(cfgs || []);
            setActiveForwards(active || []);
            setError(null);
        } catch (err) {
            console.error('Failed to fetch port forwards:', err);
            setError(err);
        } finally {
            setLoading(false);
        }
    }, [contextFilter, isVisible]);

    // Initial fetch
    useEffect(() => {
        fetchData();
    }, [fetchData]);

    // Create a subscriber callback using refs to avoid stale closures
    const subscriberRef = useRef(null);
    const contextFilterRef = useRef(contextFilter);
    contextFilterRef.current = contextFilter;

    subscriberRef.current = useCallback((event) => {
        switch (event.type) {
            case 'config_added':
                if (event.config) {
                    setConfigs(prev => {
                        // Avoid duplicates
                        if (prev.find(c => c.id === event.config.id)) return prev;
                        // Apply context filter if set
                        if (contextFilterRef.current && event.config.context !== contextFilterRef.current) return prev;
                        return [...prev, event.config];
                    });
                }
                break;
            case 'config_updated':
                if (event.config) {
                    setConfigs(prev => prev.map(c =>
                        c.id === event.config.id ? event.config : c
                    ));
                }
                break;
            case 'config_removed':
                setConfigs(prev => prev.filter(c => c.id !== event.configId));
                setActiveForwards(prev => prev.filter(af => af.config?.id !== event.configId));
                break;
            case 'started':
            case 'error':
                // Update active forwards status
                setConfigs(currentConfigs => {
                    setActiveForwards(prev => {
                        const existing = prev.find(af => af.config?.id === event.configId);
                        if (existing) {
                            return prev.map(af =>
                                af.config?.id === event.configId
                                    ? { ...af, status: event.status, error: event.error || '' }
                                    : af
                            );
                        } else {
                            const cfg = currentConfigs.find(c => c.id === event.configId);
                            if (cfg) {
                                return [...prev, {
                                    config: cfg,
                                    status: event.status,
                                    error: event.error || '',
                                    startedAt: new Date().toISOString()
                                }];
                            }
                        }
                        return prev;
                    });
                    return currentConfigs;
                });
                break;
            case 'stopped':
                setActiveForwards(prev => prev.filter(af => af.config?.id !== event.configId));
                break;
        }
    }, []);

    // Subscribe to global event system
    useEffect(() => {
        if (!window.runtime || !isVisible) return;

        // Create wrapper that calls through ref (avoids stale closure)
        const subscriber = (event) => {
            if (subscriberRef.current) {
                subscriberRef.current(event);
            }
        };

        subscribers.add(subscriber);

        // Set up global handler if not already done
        if (!globalEventHandler) {
            globalEventHandler = (event) => {
                console.log('Port forward event:', event);
                subscribers.forEach(sub => sub(event));
            };
            window.runtime.EventsOn('port-forward-event', globalEventHandler);
        }

        return () => {
            subscribers.delete(subscriber);
            // Only remove global handler when no more subscribers
            if (subscribers.size === 0 && globalEventHandler) {
                window.runtime.EventsOff('port-forward-event');
                globalEventHandler = null;
            }
        };
    }, [isVisible]);

    // Add a new port forward config
    const addConfig = useCallback(async (config) => {
        try {
            const result = await AddPortForwardConfig(config);
            return result;
        } catch (err) {
            console.error('Failed to add port forward config:', err);
            throw err;
        }
    }, []);

    // Update an existing config
    const updateConfig = useCallback(async (config) => {
        try {
            await UpdatePortForwardConfig(config);
        } catch (err) {
            console.error('Failed to update port forward config:', err);
            throw err;
        }
    }, []);

    // Delete a config
    const deleteConfig = useCallback(async (configId) => {
        try {
            await DeletePortForwardConfig(configId);
        } catch (err) {
            console.error('Failed to delete port forward config:', err);
            throw err;
        }
    }, []);

    // Start a port forward
    const startForward = useCallback(async (configId) => {
        try {
            await StartPortForward(configId);
        } catch (err) {
            console.error('Failed to start port forward:', err);
            throw err;
        }
    }, []);

    // Stop a port forward
    const stopForward = useCallback(async (configId) => {
        try {
            await StopPortForward(configId);
        } catch (err) {
            console.error('Failed to stop port forward:', err);
            throw err;
        }
    }, []);

    // Get an available port
    const getAvailablePort = useCallback(async (preferred = 0) => {
        try {
            return await GetAvailablePort(preferred);
        } catch (err) {
            console.error('Failed to get available port:', err);
            throw err;
        }
    }, []);

    // Check if a config is currently active
    const isActive = useCallback((configId) => {
        return activeForwards.some(af => af.config?.id === configId && af.status === 'running');
    }, [activeForwards]);

    // Get the status of a config
    const getStatus = useCallback((configId) => {
        const af = activeForwards.find(af => af.config?.id === configId);
        return af?.status || 'stopped';
    }, [activeForwards]);

    // Get the error for a config
    const getError = useCallback((configId) => {
        const af = activeForwards.find(af => af.config?.id === configId);
        return af?.error || '';
    }, [activeForwards]);

    return {
        configs,
        activeForwards,
        loading,
        error,
        addConfig,
        updateConfig,
        deleteConfig,
        startForward,
        stopForward,
        getAvailablePort,
        isActive,
        getStatus,
        getError,
        refresh: fetchData
    };
};
