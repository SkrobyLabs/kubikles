import { useState, useEffect, useCallback, useRef } from 'react';
import {
    GetIngressForwardState,
    DetectIngressController,
    CollectIngressHostnames,
    StartIngressForward,
    StopIngressForward,
    RefreshIngressHostnames,
    GetManagedHosts
} from 'wailsjs/go/main/App';
import { main } from 'wailsjs/go/models';
import { EventsOn, EventsOff } from 'wailsjs/runtime/runtime';

// Ingress forward event structure
interface IngressForwardEvent {
    type: string;
    state?: main.IngressForwardState;
}

// Subscriber callback type
type IngressForwardSubscriber = (event: IngressForwardEvent) => void;

// Global event listener - only one instance to avoid Wails EventsOff issues
let globalEventHandler: ((event: IngressForwardEvent) => void) | null = null;
const subscribers = new Set<IngressForwardSubscriber>();

// Return type of the hook
interface UseIngressForwardReturn {
    state: main.IngressForwardState;
    detectedController: main.IngressController | null;
    detectionAttempted: boolean;
    previewHostnames: string[];
    loading: boolean;
    detecting: boolean;
    error: Error | null;
    isActive: boolean;
    isRunning: boolean;
    detectController: () => Promise<main.IngressController | null>;
    previewHosts: (namespaces?: string[]) => Promise<string[]>;
    start: (controller: main.IngressController, namespaces?: string[]) => Promise<void>;
    stop: () => Promise<void>;
    refreshHostnames: (namespaces?: string[]) => Promise<void>;
    getManagedHosts: () => Promise<string[]>;
    resetDetection: () => void;
    refresh: () => Promise<void>;
}

/**
 * Hook for managing ingress forwarding with hosts file updates.
 * Provides functionality to port forward to the ingress controller and
 * update the local hosts file with ingress hostnames.
 */
export const useIngressForward = (): UseIngressForwardReturn => {
    const [state, setState] = useState<main.IngressForwardState>(new main.IngressForwardState({
        active: false,
        status: 'stopped',
        error: '',
        controller: undefined,
        localHttpPort: 0,
        localHttpsPort: 0,
        hostnames: [],
        portForwardIds: [],
        hostsFileUpdated: false
    }));
    const [detectedController, setDetectedController] = useState<main.IngressController | null>(null);
    const [detectionAttempted, setDetectionAttempted] = useState<boolean>(false);
    const [previewHostnames, setPreviewHostnames] = useState<string[]>([]);
    const [loading, setLoading] = useState<boolean>(false);
    const [detecting, setDetecting] = useState<boolean>(false);
    const [error, setError] = useState<Error | null>(null);

    // Fetch current state
    const fetchState = useCallback(async (): Promise<void> => {
        try {
            const currentState = await GetIngressForwardState();
            setState(currentState || new main.IngressForwardState({
                active: false,
                status: 'stopped',
                error: '',
                controller: undefined,
                localHttpPort: 0,
                localHttpsPort: 0,
                hostnames: [],
                portForwardIds: [],
                hostsFileUpdated: false
            }));
        } catch (err: any) {
            console.error('Failed to fetch ingress forward state:', err);
            setError(err instanceof Error ? err : new Error(String(err)));
        }
    }, []);

    // Initial fetch
    useEffect(() => {
        fetchState();
    }, [fetchState]);

    // Create a subscriber callback using refs to avoid stale closures
    const subscriberRef = useRef<IngressForwardSubscriber | null>(null);

    subscriberRef.current = useCallback((event: IngressForwardEvent): void => {
        console.log('Ingress forward event received:', event);
        if (event.state) {
            setState(event.state);
        }
        if (event.type === 'error' && event.state?.error) {
            setError(new Error(event.state.error));
        }
    }, []);

    // Subscribe to global event system
    useEffect(() => {
        if (!(window as any).runtime) return;

        // Create wrapper that calls through ref (avoids stale closure)
        const subscriber: IngressForwardSubscriber = (event: IngressForwardEvent) => {
            if (subscriberRef.current) {
                subscriberRef.current(event);
            }
        };

        subscribers.add(subscriber);

        // Set up global handler if not already done
        if (!globalEventHandler) {
            globalEventHandler = (event: IngressForwardEvent) => {
                console.log('Ingress forward event:', event);
                subscribers.forEach((sub: any) => {
                    try {
                        sub(event);
                    } catch (err: any) {
                        console.error('Error in ingress forward subscriber:', err);
                    }
                });
            };
            EventsOn('ingress-forward-event', globalEventHandler);
        }

        return () => {
            // Always remove subscriber, even if other cleanup fails
            subscribers.delete(subscriber);
            // Only remove global handler when no more subscribers
            if (subscribers.size === 0 && globalEventHandler) {
                try {
                    EventsOff('ingress-forward-event');
                } catch (err: any) {
                    console.error('Error removing ingress-forward-event listener:', err);
                }
                globalEventHandler = null;
            }
        };
    }, []);

    // Detect the ingress controller
    const detectController = useCallback(async (): Promise<main.IngressController | null> => {
        setDetecting(true);
        setError(null);
        try {
            const controller = await DetectIngressController();
            setDetectedController(controller);
            setDetectionAttempted(true);
            return controller;
        } catch (err: any) {
            console.error('Failed to detect ingress controller:', err);
            setError(err instanceof Error ? err : new Error(String(err)));
            setDetectedController(null);
            setDetectionAttempted(true);
            return null;
        } finally {
            setDetecting(false);
        }
    }, []);

    // Preview hostnames that will be added to hosts file
    const previewHosts = useCallback(async (namespaces: string[] = []): Promise<string[]> => {
        try {
            const hostnames = await CollectIngressHostnames(namespaces);
            setPreviewHostnames(hostnames || []);
            return hostnames || [];
        } catch (err: any) {
            console.error('Failed to collect ingress hostnames:', err);
            setError(err instanceof Error ? err : new Error(String(err)));
            return [];
        }
    }, []);

    // Start ingress forwarding
    const start = useCallback(async (controller: main.IngressController, namespaces: string[] = []): Promise<void> => {
        setLoading(true);
        setError(null);
        try {
            await StartIngressForward(controller, namespaces);
            await fetchState();
        } catch (err: any) {
            console.error('Failed to start ingress forward:', err);
            setError(err instanceof Error ? err : new Error(String(err)));
            throw err;
        } finally {
            setLoading(false);
        }
    }, [fetchState]);

    // Stop ingress forwarding
    const stop = useCallback(async (): Promise<void> => {
        setLoading(true);
        setError(null);
        try {
            await StopIngressForward();
            await fetchState();
            setPreviewHostnames([]);
        } catch (err: any) {
            console.error('Failed to stop ingress forward:', err);
            setError(err instanceof Error ? err : new Error(String(err)));
            throw err;
        } finally {
            setLoading(false);
        }
    }, [fetchState]);

    // Refresh hostnames (re-collect and update hosts file)
    const refreshHostnames = useCallback(async (namespaces: string[] = []): Promise<void> => {
        setLoading(true);
        setError(null);
        try {
            await RefreshIngressHostnames(namespaces);
            await fetchState();
        } catch (err: any) {
            console.error('Failed to refresh ingress hostnames:', err);
            setError(err instanceof Error ? err : new Error(String(err)));
            throw err;
        } finally {
            setLoading(false);
        }
    }, [fetchState]);

    // Get currently managed hosts
    const getManagedHosts = useCallback(async (): Promise<string[]> => {
        try {
            return await GetManagedHosts();
        } catch (err: any) {
            console.error('Failed to get managed hosts:', err);
            return [];
        }
    }, []);

    // Reset detection state (for retrying)
    const resetDetection = useCallback((): void => {
        setDetectedController(null);
        setDetectionAttempted(false);
        setPreviewHostnames([]);
        setError(null);
    }, []);

    return {
        // State
        state,
        detectedController,
        detectionAttempted,
        previewHostnames,
        loading,
        detecting,
        error,
        // Computed
        isActive: state.active,
        isRunning: state.status === 'running',
        // Actions
        detectController,
        previewHosts,
        start,
        stop,
        refreshHostnames,
        getManagedHosts,
        resetDetection,
        refresh: fetchState
    };
};
