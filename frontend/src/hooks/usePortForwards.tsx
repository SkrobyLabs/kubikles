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
} from 'wailsjs/go/main/App';
import { main } from 'wailsjs/go/models';
import { EventsOn, EventsOff } from 'wailsjs/runtime/runtime';

// Port forward event types
type PortForwardEventType = 'config_added' | 'config_updated' | 'config_removed' | 'started' | 'error' | 'stopped';

// Port forward event structure
interface PortForwardEvent {
    type: PortForwardEventType;
    configId?: string;
    config?: main.PortForwardConfig;
    status?: string;
    error?: string;
}

// Subscriber callback type
type PortForwardSubscriber = (event: PortForwardEvent) => void;

// Global event listener - only one instance to avoid Wails EventsOff issues
let globalEventHandler: ((event: PortForwardEvent) => void) | null = null;
const subscribers = new Set<PortForwardSubscriber>();

// Return type of the hook
interface UsePortForwardsReturn {
    configs: main.PortForwardConfig[];
    activeForwards: main.ActivePortForward[];
    loading: boolean;
    error: Error | null;
    addConfig: (config: main.PortForwardConfig) => Promise<main.PortForwardConfig>;
    updateConfig: (config: main.PortForwardConfig) => Promise<void>;
    deleteConfig: (configId: string) => Promise<void>;
    startForward: (configId: string) => Promise<void>;
    stopForward: (configId: string) => Promise<void>;
    getAvailablePort: (preferred?: number) => Promise<number>;
    isActive: (configId: string) => boolean;
    getStatus: (configId: string) => string;
    getError: (configId: string) => string;
    refresh: () => Promise<void>;
}

/**
 * Hook for managing port forwards.
 * Port forwards persist across context switches and app restarts.
 */
export const usePortForwards = (contextFilter: string = '', isVisible: boolean = true): UsePortForwardsReturn => {
    const [configs, setConfigs] = useState<main.PortForwardConfig[]>([]);
    const [activeForwards, setActiveForwards] = useState<main.ActivePortForward[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);

    // Fetch configs and active forwards
    const fetchData = useCallback(async (): Promise<void> => {
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
        } catch (err: any) {
            console.error('Failed to fetch port forwards:', err);
            setError(err instanceof Error ? err : new Error(String(err)));
        } finally {
            setLoading(false);
        }
    }, [contextFilter, isVisible]);

    // Initial fetch
    useEffect(() => {
        fetchData();
    }, [fetchData]);

    // Create a subscriber callback using refs to avoid stale closures
    const subscriberRef = useRef<PortForwardSubscriber | null>(null);
    const contextFilterRef = useRef<string>(contextFilter);
    contextFilterRef.current = contextFilter;

    subscriberRef.current = useCallback((event: PortForwardEvent): void => {
        switch (event.type) {
            case 'config_added':
                if (event.config) {
                    setConfigs(prev => {
                        // Avoid duplicates
                        if (prev.find((c: any) => c.id === event.config!.id)) return prev;
                        // Apply context filter if set
                        if (contextFilterRef.current && event.config!.context !== contextFilterRef.current) return prev;
                        return [...prev, event.config!];
                    });
                }
                break;
            case 'config_updated':
                if (event.config) {
                    setConfigs(prev => prev.map((c: any) =>
                        c.id === event.config!.id ? event.config! : c
                    ));
                }
                break;
            case 'config_removed':
                setConfigs(prev => prev.filter((c: any) => c.id !== event.configId));
                setActiveForwards(prev => prev.filter((af: any) => af.config?.id !== event.configId));
                break;
            case 'started':
            case 'error':
                // Update active forwards status
                setConfigs(currentConfigs => {
                    setActiveForwards(prev => {
                        const existing = prev.find((af: any) => af.config?.id === event.configId);
                        if (existing) {
                            return prev.map((af: any) =>
                                af.config?.id === event.configId
                                    ? { ...af, status: event.status || af.status, error: event.error || '' } as any
                                    : af
                            );
                        } else {
                            const cfg = currentConfigs.find((c: any) => c.id === event.configId);
                            if (cfg && event.status) {
                                const activeForward = new main.ActivePortForward({
                                    config: cfg,
                                    status: event.status,
                                    error: event.error || '',
                                    startedAt: new Date().toISOString()
                                });
                                return [...prev, activeForward];
                            }
                        }
                        return prev;
                    });
                    return currentConfigs;
                });
                break;
            case 'stopped':
                setActiveForwards(prev => prev.filter((af: any) => af.config?.id !== event.configId));
                break;
        }
    }, []);

    // Subscribe to global event system
    useEffect(() => {
        if (!(window as any).runtime || !isVisible) return;

        // Create wrapper that calls through ref (avoids stale closure)
        const subscriber: PortForwardSubscriber = (event: PortForwardEvent) => {
            if (subscriberRef.current) {
                subscriberRef.current(event);
            }
        };

        subscribers.add(subscriber);

        // Set up global handler if not already done
        if (!globalEventHandler) {
            globalEventHandler = (event: PortForwardEvent) => {
                console.log('Port forward event:', event);
                subscribers.forEach((sub: any) => {
                    try {
                        sub(event);
                    } catch (err: any) {
                        console.error('Error in port forward subscriber:', err);
                    }
                });
            };
            EventsOn('port-forward-event', globalEventHandler);
        }

        return () => {
            // Always remove subscriber, even if other cleanup fails
            subscribers.delete(subscriber);
            // Only remove global handler when no more subscribers
            if (subscribers.size === 0 && globalEventHandler) {
                try {
                    EventsOff('port-forward-event');
                } catch (err: any) {
                    console.error('Error removing port-forward-event listener:', err);
                }
                globalEventHandler = null;
            }
        };
    }, [isVisible]);

    // Add a new port forward config
    const addConfig = useCallback(async (config: main.PortForwardConfig): Promise<main.PortForwardConfig> => {
        try {
            const result = await AddPortForwardConfig(config);
            return result;
        } catch (err: any) {
            console.error('Failed to add port forward config:', err);
            throw err;
        }
    }, []);

    // Update an existing config
    const updateConfig = useCallback(async (config: main.PortForwardConfig): Promise<void> => {
        try {
            await UpdatePortForwardConfig(config);
        } catch (err: any) {
            console.error('Failed to update port forward config:', err);
            throw err;
        }
    }, []);

    // Delete a config
    const deleteConfig = useCallback(async (configId: string): Promise<void> => {
        try {
            await DeletePortForwardConfig(configId);
        } catch (err: any) {
            console.error('Failed to delete port forward config:', err);
            throw err;
        }
    }, []);

    // Start a port forward
    const startForward = useCallback(async (configId: string): Promise<void> => {
        try {
            await StartPortForward(configId);
        } catch (err: any) {
            console.error('Failed to start port forward:', err);
            throw err;
        }
    }, []);

    // Stop a port forward
    const stopForward = useCallback(async (configId: string): Promise<void> => {
        try {
            await StopPortForward(configId);
        } catch (err: any) {
            console.error('Failed to stop port forward:', err);
            throw err;
        }
    }, []);

    // Get an available port
    const getAvailablePort = useCallback(async (preferred: number = 0): Promise<number> => {
        try {
            return await GetAvailablePort(preferred);
        } catch (err: any) {
            console.error('Failed to get available port:', err);
            throw err;
        }
    }, []);

    // Check if a config is currently active
    const isActive = useCallback((configId: string): boolean => {
        return activeForwards.some((af: any) => af.config?.id === configId && af.status === 'running');
    }, [activeForwards]);

    // Get the status of a config
    const getStatus = useCallback((configId: string): string => {
        const af = activeForwards.find((af: any) => af.config?.id === configId);
        return af?.status || 'stopped';
    }, [activeForwards]);

    // Get the error for a config
    const getError = useCallback((configId: string): string => {
        const af = activeForwards.find((af: any) => af.config?.id === configId);
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
