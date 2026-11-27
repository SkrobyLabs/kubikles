import React, { useEffect, useRef } from 'react';
import { Terminal as XTerm } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { getClientId } from '~/lib/wailsjs-adapter/runtime/runtime';
import { StartTerminalSession, SendTerminalInput, ResizeTerminal, CloseTerminalSession } from 'wailsjs/go/main/App';
import { createDedupedSizeSender, createFitScheduler } from '~/utils/terminalResize';
import 'xterm/css/xterm.css';

// Get theme colors from CSS variables
const getThemeColors = () => {
    const root = document.documentElement;
    const getVar = (name: string, fallback: string) => getComputedStyle(root).getPropertyValue(name).trim() || fallback;
    return {
        background: getVar('--color-background', '#1e1e1e'),
        foreground: getVar('--color-text', '#cccccc'),
        cursor: getVar('--color-primary', '#007acc'),
        cursorAccent: getVar('--color-background', '#1e1e1e'),
        selectionBackground: getVar('--color-primary', '#007acc') + '40',
    };
};

const Terminal = ({ namespace, pod, container, context, command, onClose }: { namespace: any; pod: any; container: any; context: any; command: any; onClose: any }) => {
    const terminalRef = useRef<HTMLDivElement>(null);
    const xtermRef = useRef<any>(null);
    const sessionIdRef = useRef<any>(null);
    const fitAddonRef = useRef<any>(null);
    const webglAddonRef = useRef<any>(null);
    const onCloseRef = useRef(onClose);
    const cleanupTimeoutRef = useRef<any>(null);

    // Keep onClose ref up to date without triggering effect re-runs
    useEffect(() => {
        onCloseRef.current = onClose;
    }, [onClose]);

    useEffect(() => {
        // Cancel any pending cleanup from React Strict Mode double-invocation
        if (cleanupTimeoutRef.current) {
            clearTimeout(cleanupTimeoutRef.current);
            cleanupTimeoutRef.current = null;
        }

        // Initialize xterm with theme colors
        const themeColors = getThemeColors();
        const term = new XTerm({
            cursorBlink: true,
            theme: themeColors,
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Ubuntu Mono", "DejaVu Sans Mono", "Liberation Mono", "Courier New", monospace',
        });

        const fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        fitAddonRef.current = fitAddon;

        term.open(terminalRef.current!);

        // WebKit caps live WebGL contexts, so hidden keepAlive terminals release
        // theirs (DOM renderer is used meanwhile) and re-attach on show.
        const attachWebgl = () => {
            if (webglAddonRef.current) return;
            try {
                const webglAddon = new WebglAddon();
                (webglAddon as any).onContextLost(() => {
                    console.warn('WebGL context lost, falling back to default renderer');
                    webglAddon.dispose();
                    if (webglAddonRef.current === webglAddon) webglAddonRef.current = null;
                });
                term.loadAddon(webglAddon);
                webglAddonRef.current = webglAddon;
            } catch (e: any) {
                console.warn('WebGL addon failed to load, using default renderer:', (e as any).message);
            }
        };
        const detachWebgl = () => {
            if (!webglAddonRef.current) return;
            try { webglAddonRef.current.dispose(); } catch { /* already lost */ }
            webglAddonRef.current = null;
        };
        attachWebgl();

        // Zero-size container means a hidden (display:none) keepAlive tab.
        const isShown = () => !!terminalRef.current && terminalRef.current.clientWidth > 0 && terminalRef.current.clientHeight > 0;

        // Resize path: observers/window events only request a fit (debounced);
        // size changes reach the backend solely through term.onResize, deduped.
        const sizeSender = createDedupedSizeSender((cols, rows) => {
            if (sessionIdRef.current) ResizeTerminal(sessionIdRef.current, cols, rows);
        });
        const fitScheduler = createFitScheduler(() => {
            try {
                fitAddon.fit();
            } catch (e: any) {
                console.error('Terminal fit failed', e);
            }
        }, isShown, 100);
        let wasShown = isShown();
        const syncGeometry = () => {
            const shown = isShown();
            if (!shown) {
                if (wasShown) detachWebgl();
                wasShown = false;
                return;
            }
            if (!wasShown) {
                attachWebgl();
                wasShown = true;
                try { term.refresh(0, term.rows - 1); } catch { /* disposed */ }
            }
            fitScheduler.request();
        };
        const timers: ReturnType<typeof setTimeout>[] = [];
        // Slight delay to ensure container has dimensions
        timers.push(setTimeout(syncGeometry, 100));

        xtermRef.current = term;

        // Start terminal session via Wails
        // Pass clientId for server mode session cleanup on disconnect
        const clientId = getClientId() || '';
        StartTerminalSession({ namespace, pod, container, context, command, clientId })
            .then((sessionId: any) => {
                sessionIdRef.current = sessionId;
                term.write('\r\n\x1b[32mConnected to terminal...\x1b[0m\r\n');

                // Send initial resize after connection
                sizeSender.reset();
                timers.push(setTimeout(() => {
                    if (isShown()) sizeSender.send(term.cols, term.rows);
                }, 150));
            })
            .catch((err: any) => {
                term.write(`\r\n\x1b[31mFailed to start terminal: ${err}\x1b[0m\r\n`);
            });

        // Listen for terminal output events
        const handleTerminalOutput = (event: any) => {
            if (!sessionIdRef.current || event.sessionId !== sessionIdRef.current) return;

            if (event.done) {
                term.write('\r\n\x1b[33mProcess exited. Terminal disconnected.\x1b[0m\r\n');
                sessionIdRef.current = null;
                return;
            }

            if (event.error) {
                term.write(`\r\n\x1b[31mError: ${event.error}\x1b[0m\r\n`);
                return;
            }

            if (event.data) {
                term.write(event.data);
            }
        };

        const cancelTerminalOutput = EventsOn('terminal:output', handleTerminalOutput);

        // Let the browser handle Ctrl+C (copy when selected) and Ctrl+V (paste) on Windows/Linux
        term.attachCustomKeyEventHandler((e) => {
            if (e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey) {
                // Ctrl+C with selection → copy; without selection → SIGINT (let xterm handle)
                if (e.key === 'c' && term.hasSelection()) return false;
                // Ctrl+V → paste
                if (e.key === 'v') return false;
            }
            return true;
        });

        // Send input to terminal
        term.onData((data) => {
            if (sessionIdRef.current) {
                SendTerminalInput(sessionIdRef.current, data);
            }
        });

        window.addEventListener('resize', syncGeometry);
        term.onResize(({ cols, rows }) => sizeSender.send(cols, rows));

        const resizeObserver = new ResizeObserver(syncGeometry);
        const intersectionObserver = new IntersectionObserver(syncGeometry);
        if (terminalRef.current) {
            resizeObserver.observe(terminalRef.current);
            intersectionObserver.observe(terminalRef.current);
        }

        // Cleanup
        return () => {
            window.removeEventListener('resize', syncGeometry);
            resizeObserver.disconnect();
            intersectionObserver.disconnect();
            fitScheduler.cancel();
            timers.forEach(clearTimeout);
            cancelTerminalOutput();

            // Close the terminal session
            if (sessionIdRef.current) {
                CloseTerminalSession(sessionIdRef.current);
                sessionIdRef.current = null;
            }

            // Dispose WebGL addon before terminal
            detachWebgl();
            term.dispose();

            // Delay onClose to handle React Strict Mode double-invocation
            // If the effect re-runs (strict mode), this timeout will be cancelled
            if (onCloseRef.current) {
                cleanupTimeoutRef.current = setTimeout(() => {
                    if (onCloseRef.current) {
                        onCloseRef.current();
                    }
                }, 100);
            }
        };
    }, [namespace, pod, container, context, command]);

    return (
        <div className="h-full w-full bg-background p-2 overflow-hidden relative">
            <style>{`
                .xterm { cursor: text; position: relative; user-select: none; -ms-user-select: none; -webkit-user-select: none; }
                .xterm.focus, .xterm:focus { outline: none; }
                .xterm .xterm-helpers { position: absolute; z-index: 5; }
                .xterm .xterm-helper-textarea { position: absolute; opacity: 0; z-index: -5; top: 0; left: 0; width: 0; height: 0; overflow: hidden; margin: 0; padding: 0; border: 0; }
                .xterm .xterm-viewport { background-color: transparent !important; overflow-y: scroll; cursor: default; position: absolute; right: 0; left: 0; top: 0; bottom: 0; }
                .xterm .xterm-screen { position: relative; }
                .xterm .xterm-screen canvas { position: absolute; left: 0; top: 0; }
                .xterm .xterm-scroll-area { visibility: hidden; }
                .xterm-char-measure-element { display: inline-block; visibility: hidden; position: absolute; top: 0; left: -9999em; line-height: normal; }
                .xterm.enable-mouse-events { cursor: default; }
                .xterm .xterm-cursor-pointer { cursor: pointer; }
                .xterm .xterm-cursor-text { cursor: text; }
            `}</style>
            <div
                ref={terminalRef}
                className="h-full w-full"
            />
        </div>
    );
};

export default Terminal;
