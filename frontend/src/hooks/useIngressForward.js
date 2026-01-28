import { useState, useEffect, useCallback, useRef } from 'react';
import {
    GetIngressForwardState,
    DetectIngressController,
    CollectIngressHostnames,
    StartIngressForward,
    StopIngressForward,
    RefreshIngressHostnames,
    GetManagedHosts
} from '../../wailsjs/go/main/App';
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime';

// Global event listener - only one instance to avoid Wails EventsOff issues
let globalEventHandler = null;
const subscribers = new Set();

/**
 * Hook for managing ingress forwarding with hosts file updates.
 * Provides functionality to port forward to the ingress controller and
 * update the local hosts file with ingress hostnames.
 */
export const useIngressForward = () => {
    const [state, setState] = useState({
        active: false,
        status: 'stopped',
        error: '',
        controller: null,
        localHttpPort: 0,
        localHttpsPort: 0,
        hostnames: [],
        portForwardIds: [],
        hostsFileUpdated: false
    });
    const [detectedController, setDetectedController] = useState(null);
    const [detectionAttempted, setDetectionAttempted] = useState(false);
    const [previewHostnames, setPreviewHostnames] = useState([]);
    const [loading, setLoading] = useState(false);
    const [detecting, setDetecting] = useState(false);
    const [error, setError] = useState(null);

    // Fetch current state
    const fetchState = useCallback(async () => {
        try {
            const currentState = await GetIngressForwardState();
            setState(currentState || {
                active: false,
                status: 'stopped',
                error: '',
                controller: null,
                localHttpPort: 0,
                localHttpsPort: 0,
                hostnames: [],
                portForwardIds: [],
                hostsFileUpdated: false
            });
        } catch (err) {
            console.error('Failed to fetch ingress forward state:', err);
            setError(err);
        }
    }, []);

    // Initial fetch
    useEffect(() => {
        fetchState();
    }, [fetchState]);

    // Create a subscriber callback using refs to avoid stale closures
    const subscriberRef = useRef(null);

    subscriberRef.current = useCallback((event) => {
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
        if (!window.runtime) return;

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
                console.log('Ingress forward event:', event);
                subscribers.forEach(sub => sub(event));
            };
            EventsOn('ingress-forward-event', globalEventHandler);
        }

        return () => {
            subscribers.delete(subscriber);
            // Only remove global handler when no more subscribers
            if (subscribers.size === 0 && globalEventHandler) {
                EventsOff('ingress-forward-event');
                globalEventHandler = null;
            }
        };
    }, []);

    // Detect the ingress controller
    const detectController = useCallback(async () => {
        setDetecting(true);
        setError(null);
        try {
            const controller = await DetectIngressController();
            setDetectedController(controller);
            setDetectionAttempted(true);
            return controller;
        } catch (err) {
            console.error('Failed to detect ingress controller:', err);
            setError(err);
            setDetectedController(null);
            setDetectionAttempted(true);
            return null;
        } finally {
            setDetecting(false);
        }
    }, []);

    // Preview hostnames that will be added to hosts file
    const previewHosts = useCallback(async (namespaces = []) => {
        try {
            const hostnames = await CollectIngressHostnames(namespaces);
            setPreviewHostnames(hostnames || []);
            return hostnames || [];
        } catch (err) {
            console.error('Failed to collect ingress hostnames:', err);
            setError(err);
            return [];
        }
    }, []);

    // Start ingress forwarding
    const start = useCallback(async (controller, namespaces = []) => {
        setLoading(true);
        setError(null);
        try {
            await StartIngressForward(controller, namespaces);
            await fetchState();
        } catch (err) {
            console.error('Failed to start ingress forward:', err);
            setError(err);
            throw err;
        } finally {
            setLoading(false);
        }
    }, [fetchState]);

    // Stop ingress forwarding
    const stop = useCallback(async () => {
        setLoading(true);
        setError(null);
        try {
            await StopIngressForward();
            await fetchState();
            setPreviewHostnames([]);
        } catch (err) {
            console.error('Failed to stop ingress forward:', err);
            setError(err);
            throw err;
        } finally {
            setLoading(false);
        }
    }, [fetchState]);

    // Refresh hostnames (re-collect and update hosts file)
    const refreshHostnames = useCallback(async (namespaces = []) => {
        setLoading(true);
        setError(null);
        try {
            await RefreshIngressHostnames(namespaces);
            await fetchState();
        } catch (err) {
            console.error('Failed to refresh ingress hostnames:', err);
            setError(err);
            throw err;
        } finally {
            setLoading(false);
        }
    }, [fetchState]);

    // Get currently managed hosts
    const getManagedHosts = useCallback(async () => {
        try {
            return await GetManagedHosts();
        } catch (err) {
            console.error('Failed to get managed hosts:', err);
            return [];
        }
    }, []);

    // Reset detection state (for retrying)
    const resetDetection = useCallback(() => {
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
