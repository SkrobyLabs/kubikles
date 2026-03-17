import React, { useEffect, useRef } from 'react';
import { Terminal as XTerm } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import { EventsOn } from 'wailsjs/runtime/runtime';
import { getClientId } from '~/lib/wailsjs-adapter/runtime/runtime';
import { StartTerminalSession, SendTerminalInput, ResizeTerminal, CloseTerminalSession } from 'wailsjs/go/main/App';
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

        // Load WebGL addon for GPU-accelerated rendering (Metal on macOS)
        try {
            const webglAddon = new WebglAddon();
            (webglAddon as any).onContextLost(() => {
                console.warn('WebGL context lost, falling back to canvas renderer');
                webglAddon.dispose();
                webglAddonRef.current = null;
            });
            term.loadAddon(webglAddon);
            webglAddonRef.current = webglAddon;
            console.log('Terminal using WebGL renderer (GPU accelerated)');
        } catch (e: any) {
            console.warn('WebGL addon failed to load, using canvas renderer:', (e as any).message);
        }

        // Slight delay to ensure container has dimensions
        setTimeout(() => {
            try {
                fitAddon.fit();
                console.log("Terminal fit complete", fitAddon.proposeDimensions());
            } catch (e: any) {
                console.error("Terminal fit failed", e);
            }
        }, 100);

        xtermRef.current = term;

        // Start terminal session via Wails
        // Pass clientId for server mode session cleanup on disconnect
        const clientId = getClientId() || '';
        StartTerminalSession({ namespace, pod, container, context, command, clientId })
            .then((sessionId: any) => {
                sessionIdRef.current = sessionId;
                term.write('\r\n\x1b[32mConnected to terminal...\x1b[0m\r\n');

                // Send initial resize after connection
                setTimeout(() => {
                    const dims = fitAddon.proposeDimensions();
                    if (dims && sessionIdRef.current) {
                        ResizeTerminal(sessionIdRef.current, dims.cols, dims.rows);
                    }
                }, 150);
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

        // Handle resize
        const sendResizeToBackend = () => {
            const dims = fitAddon.proposeDimensions();
            if (dims && sessionIdRef.current) {
                ResizeTerminal(sessionIdRef.current, dims.cols, dims.rows);
            }
        };

        const handleResize = () => {
            fitAddon.fit();
            sendResizeToBackend();
        };
        window.addEventListener('resize', handleResize);

        // Listen for terminal resize events from xterm
        term.onResize(({ cols, rows }) => {
            if (sessionIdRef.current) {
                ResizeTerminal(sessionIdRef.current, cols, rows);
            }
        });

        // Cleanup
        return () => {
            window.removeEventListener('resize', handleResize);
            cancelTerminalOutput();

            // Close the terminal session
            if (sessionIdRef.current) {
                CloseTerminalSession(sessionIdRef.current);
                sessionIdRef.current = null;
            }

            // Dispose WebGL addon before terminal
            if (webglAddonRef.current) {
                webglAddonRef.current.dispose();
                webglAddonRef.current = null;
            }
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

    // Re-fit when the container size changes or becomes visible
    useEffect(() => {
        const fit = () => {
            if (fitAddonRef.current && terminalRef.current) {
                try {
                    // Check dimensions before fitting
                    const { clientWidth, clientHeight } = terminalRef.current;

                    if (clientWidth > 0 && clientHeight > 0) {
                        fitAddonRef.current.fit();
                    }
                } catch (e: any) {
                    console.error("Fit failed", e);
                }
            }
        };

        const resizeObserver = new ResizeObserver(() => {
            requestAnimationFrame(fit);
        });

        const intersectionObserver = new IntersectionObserver((entries) => {
            if (entries[0].isIntersecting) {
                setTimeout(fit, 100);
            }
        });

        if (terminalRef.current) {
            resizeObserver.observe(terminalRef.current);
            intersectionObserver.observe(terminalRef.current);
        }

        return () => {
            resizeObserver.disconnect();
            intersectionObserver.disconnect();
        };
    }, []);

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
