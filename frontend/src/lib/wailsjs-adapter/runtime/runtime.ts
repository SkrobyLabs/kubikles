// @ts-nocheck
// Wails Runtime Adapter - handles events and window functions for both desktop and server modes
// SYNC: This file mirrors wailsjs/runtime/runtime.js from Wails v2
// See: https://github.com/wailsapp/wails/blob/master/v2/internal/frontend/runtime/runtime.js
//
// ACTIVELY USED functions (keep implementations current):
//   - EventsOn, EventsOff, EventsOnMultiple, EventsOnce, EventsOffAll
//   - BrowserOpenURL
//   - Environment
//   - isInServerMode
//   - OnFileDrop, OnFileDropOff
//   - WindowToggleMaximise
//
// UNUSED functions (stubs for API compatibility - no-ops in server mode):
//   - Log*, Window* (except WindowToggleMaximise), Screen*, Clipboard*, Quit, Hide, Show, etc.

// =============================================================================
// MODE DETECTION
// =============================================================================

const isServerMode = () => typeof window.runtime === 'undefined';

// Export helper for mode detection
export function isInServerMode() {
    return isServerMode();
}

// =============================================================================
// WEBSOCKET CONNECTION (Server Mode Only)
// =============================================================================

let ws = null;
let wsConnecting = false;
let wsReconnectTimer = null;
let currentClientId = null; // Server-assigned client ID for session tracking
const eventListeners = new Map(); // eventName -> Set of callbacks

// Get the current WebSocket client ID (server mode only, used for session ownership)
export function getClientId() {
    return currentClientId;
}

function connectWebSocket() {
    if (!isServerMode()) return;
    if (ws?.readyState === WebSocket.OPEN || ws?.readyState === WebSocket.CONNECTING || wsConnecting) return;

    wsConnecting = true;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/ws`;

    try {
        ws = new WebSocket(wsUrl);

        ws.onopen = () => {
            console.log('[WS] Connected');
            wsConnecting = false;
            if (wsReconnectTimer) {
                clearTimeout(wsReconnectTimer);
                wsReconnectTimer = null;
            }
        };

        ws.onmessage = (event) => {
            try {
                const msg = JSON.parse(event.data);
                const eventName = msg.name;
                if (eventName && (msg.type === 'event' || !msg.type)) {
                    // Capture client ID from connected event for session ownership
                    if (eventName === 'connected' && msg.data?.clientId) {
                        currentClientId = msg.data.clientId;
                        console.log('[WS] Client ID:', currentClientId);
                    }
                    const listeners = eventListeners.get(eventName);
                    if (listeners) {
                        listeners.forEach(callback => {
                            try {
                                callback(msg.data);
                            } catch (err) {
                                console.error(`[WS] Event handler error for ${eventName}:`, err);
                            }
                        });
                    }
                }
            } catch (err) {
                console.error('[WS] Message parse error:', err);
            }
        };

        ws.onclose = () => {
            console.log('[WS] Disconnected, reconnecting in 2s...');
            ws = null;
            wsConnecting = false;
            wsReconnectTimer = setTimeout(connectWebSocket, 2000);
        };

        ws.onerror = (err) => {
            console.error('[WS] Error:', err);
            wsConnecting = false;
        };
    } catch (err) {
        console.error('[WS] Connection error:', err);
        wsConnecting = false;
        wsReconnectTimer = setTimeout(connectWebSocket, 2000);
    }
}

// Initialize WebSocket on module load in server mode
if (typeof window !== 'undefined' && isServerMode()) {
    connectWebSocket();
}

// =============================================================================
// EVENT FUNCTIONS (Actively Used)
// =============================================================================

export function EventsOnMultiple(eventName, callback, maxCallbacks) {
    if (isServerMode()) {
        if (!eventListeners.has(eventName)) {
            eventListeners.set(eventName, new Set());
        }
        const actualCallback = (maxCallbacks > 0)
            ? (() => {
                let remaining = maxCallbacks;
                return (data) => {
                    if (remaining <= 0) return;
                    remaining--;
                    callback(data);
                    if (remaining <= 0) {
                        eventListeners.get(eventName)?.delete(actualCallback);
                    }
                };
            })()
            : callback;
        eventListeners.get(eventName).add(actualCallback);
        connectWebSocket();
        // Return cleanup that only removes this specific callback
        return () => eventListeners.get(eventName)?.delete(actualCallback);
    }
    return window.runtime.EventsOnMultiple(eventName, callback, maxCallbacks);
}

export function EventsOn(eventName, callback) {
    return EventsOnMultiple(eventName, callback, -1);
}

export function EventsOff(eventName, ...additionalEventNames) {
    if (isServerMode()) {
        [eventName, ...additionalEventNames].forEach(name => eventListeners.delete(name));
        return;
    }
    return window.runtime.EventsOff(eventName, ...additionalEventNames);
}

export function EventsOffAll() {
    if (isServerMode()) {
        eventListeners.clear();
        return;
    }
    return window.runtime.EventsOffAll();
}

// Test-only: dispatch an event locally to all registered listeners (server mode only).
// Mirrors the ws.onmessage dispatch path for unit testing event subscription lifecycle.
export function _testDispatchEvent(eventName, data) {
    const listeners = eventListeners.get(eventName);
    if (listeners) {
        listeners.forEach(cb => { try { cb(data); } catch (_) {} });
    }
}

export function EventsOnce(eventName, callback) {
    return EventsOnMultiple(eventName, callback, 1);
}

export function EventsEmit(eventName, ...args) {
    if (isServerMode()) {
        fetch('/api/emit', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ name: eventName, data: args[0] }),
        }).catch(err => console.error('[EventsEmit] Error:', err));
        return;
    }
    return window.runtime.EventsEmit(eventName, ...args);
}

// =============================================================================
// BROWSER FUNCTIONS (Actively Used)
// =============================================================================

export function BrowserOpenURL(url) {
    if (isServerMode()) {
        window.open(url, '_blank');
        return;
    }
    window.runtime.BrowserOpenURL(url);
}

export function Environment() {
    if (isServerMode()) {
        return Promise.resolve({
            buildType: 'server',
            platform: navigator.platform,
            arch: 'unknown',
        });
    }
    return window.runtime.Environment();
}

// =============================================================================
// FILE DROP (Actively Used)
// =============================================================================

export function OnFileDrop(callback, useDropTarget) {
    if (isServerMode()) {
        const handler = (e) => {
            e.preventDefault();
            if (e.dataTransfer?.files?.length) {
                const paths = Array.from(e.dataTransfer.files).map(f => f.name);
                callback(e.clientX, e.clientY, paths);
            }
        };
        document.addEventListener('drop', handler);
        document.addEventListener('dragover', (e) => e.preventDefault());
        return () => document.removeEventListener('drop', handler);
    }
    return window.runtime.OnFileDrop(callback, useDropTarget);
}

export function OnFileDropOff() {
    if (isServerMode()) return;
    return window.runtime.OnFileDropOff();
}

// =============================================================================
// WINDOW FUNCTIONS (Only WindowToggleMaximise is used)
// =============================================================================

export function WindowToggleMaximise() {
    if (isServerMode()) return;
    window.runtime.WindowToggleMaximise();
}

// --- Unused Window Functions (stubs for API compatibility) ---

export function WindowReload() {
    if (isServerMode()) { window.location.reload(); return; }
    window.runtime.WindowReload();
}

export function WindowReloadApp() {
    if (isServerMode()) { window.location.reload(); return; }
    window.runtime.WindowReloadApp();
}

export function WindowSetTitle(title) {
    if (isServerMode()) { document.title = title; return; }
    window.runtime.WindowSetTitle(title);
}

export function WindowFullscreen() {
    if (isServerMode()) { document.documentElement.requestFullscreen?.(); return; }
    window.runtime.WindowFullscreen();
}

export function WindowUnfullscreen() {
    if (isServerMode()) { document.exitFullscreen?.(); return; }
    window.runtime.WindowUnfullscreen();
}

export function WindowIsFullscreen() {
    if (isServerMode()) return !!document.fullscreenElement;
    return window.runtime.WindowIsFullscreen();
}

export function WindowGetSize() {
    if (isServerMode()) return { w: window.innerWidth, h: window.innerHeight };
    return window.runtime.WindowGetSize();
}

export function WindowGetPosition() {
    if (isServerMode()) return { x: 0, y: 0 };
    return window.runtime.WindowGetPosition();
}

export function WindowIsMaximised() {
    if (isServerMode()) return false;
    return window.runtime.WindowIsMaximised();
}

export function WindowIsMinimised() {
    if (isServerMode()) return false;
    return window.runtime.WindowIsMinimised();
}

export function WindowIsNormal() {
    if (isServerMode()) return true;
    return window.runtime.WindowIsNormal();
}

// No-op stubs (server mode ignores these)
export function WindowSetAlwaysOnTop(b) { if (!isServerMode()) window.runtime.WindowSetAlwaysOnTop(b); }
export function WindowSetSystemDefaultTheme() { if (!isServerMode()) window.runtime.WindowSetSystemDefaultTheme(); }
export function WindowSetLightTheme() { if (!isServerMode()) window.runtime.WindowSetLightTheme(); }
export function WindowSetDarkTheme() { if (!isServerMode()) window.runtime.WindowSetDarkTheme(); }
export function WindowCenter() { if (!isServerMode()) window.runtime.WindowCenter(); }
export function WindowSetSize(w, h) { if (!isServerMode()) window.runtime.WindowSetSize(w, h); }
export function WindowSetMaxSize(w, h) { if (!isServerMode()) window.runtime.WindowSetMaxSize(w, h); }
export function WindowSetMinSize(w, h) { if (!isServerMode()) window.runtime.WindowSetMinSize(w, h); }
export function WindowSetPosition(x, y) { if (!isServerMode()) window.runtime.WindowSetPosition(x, y); }
export function WindowHide() { if (!isServerMode()) window.runtime.WindowHide(); }
export function WindowShow() { if (!isServerMode()) window.runtime.WindowShow(); }
export function WindowMaximise() { if (!isServerMode()) window.runtime.WindowMaximise(); }
export function WindowUnmaximise() { if (!isServerMode()) window.runtime.WindowUnmaximise(); }
export function WindowMinimise() { if (!isServerMode()) window.runtime.WindowMinimise(); }
export function WindowUnminimise() { if (!isServerMode()) window.runtime.WindowUnminimise(); }
export function WindowSetBackgroundColour(R, G, B, A) { if (!isServerMode()) window.runtime.WindowSetBackgroundColour(R, G, B, A); }

// =============================================================================
// LOGGING FUNCTIONS (Unused - stubs for API compatibility)
// =============================================================================

export function LogPrint(message) { if (isServerMode()) console.log(message); else window.runtime.LogPrint(message); }
export function LogTrace(message) { if (isServerMode()) console.trace(message); else window.runtime.LogTrace(message); }
export function LogDebug(message) { if (isServerMode()) console.debug(message); else window.runtime.LogDebug(message); }
export function LogInfo(message) { if (isServerMode()) console.info(message); else window.runtime.LogInfo(message); }
export function LogWarning(message) { if (isServerMode()) console.warn(message); else window.runtime.LogWarning(message); }
export function LogError(message) { if (isServerMode()) console.error(message); else window.runtime.LogError(message); }
export function LogFatal(message) { if (isServerMode()) console.error('[FATAL]', message); else window.runtime.LogFatal(message); }

// =============================================================================
// OTHER FUNCTIONS (Unused - stubs for API compatibility)
// =============================================================================

export function ScreenGetAll() {
    if (isServerMode()) return [];
    return window.runtime.ScreenGetAll();
}

export function ClipboardGetText() {
    if (isServerMode()) return navigator.clipboard.readText();
    return window.runtime.ClipboardGetText();
}

export function ClipboardSetText(text) {
    if (isServerMode()) return navigator.clipboard.writeText(text);
    return window.runtime.ClipboardSetText(text);
}

export function CanResolveFilePaths() {
    if (isServerMode()) return false;
    return window.runtime.CanResolveFilePaths();
}

export function ResolveFilePaths(files) {
    if (isServerMode()) return Promise.resolve([]);
    return window.runtime.ResolveFilePaths(files);
}

export function Quit() {
    if (isServerMode()) { window.close(); return; }
    window.runtime.Quit();
}

export function Hide() { if (!isServerMode()) window.runtime.Hide(); }
export function Show() { if (!isServerMode()) window.runtime.Show(); }
