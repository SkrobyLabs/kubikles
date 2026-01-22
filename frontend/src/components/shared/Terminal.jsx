import React, { useEffect, useRef } from 'react';
import { Terminal as XTerm } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import { WebglAddon } from '@xterm/addon-webgl';
import 'xterm/css/xterm.css';

const Terminal = ({ url, onClose }) => {
    const terminalRef = useRef(null);
    const xtermRef = useRef(null);
    const wsRef = useRef(null);
    const fitAddonRef = useRef(null);
    const webglAddonRef = useRef(null);
    const onCloseRef = useRef(onClose);
    const cleanupTimeoutRef = useRef(null);

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

        // Initialize xterm
        const term = new XTerm({
            cursorBlink: true,
            theme: {
                background: '#1e1e1e',
                foreground: '#ffffff',
            },
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Ubuntu Mono", "DejaVu Sans Mono", "Liberation Mono", "Courier New", monospace',
        });

        const fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        fitAddonRef.current = fitAddon;

        term.open(terminalRef.current);

        // Load WebGL addon for GPU-accelerated rendering (Metal on macOS)
        try {
            const webglAddon = new WebglAddon();
            webglAddon.onContextLost(() => {
                console.warn('WebGL context lost, falling back to canvas renderer');
                webglAddon.dispose();
                webglAddonRef.current = null;
            });
            term.loadAddon(webglAddon);
            webglAddonRef.current = webglAddon;
            console.log('Terminal using WebGL renderer (GPU accelerated)');
        } catch (e) {
            console.warn('WebGL addon failed to load, using canvas renderer:', e.message);
        }

        // Slight delay to ensure container has dimensions
        setTimeout(() => {
            try {
                fitAddon.fit();
                console.log("Terminal fit complete", fitAddon.proposeDimensions());
            } catch (e) {
                console.error("Terminal fit failed", e);
            }
        }, 100);

        xtermRef.current = term;

        // Connect to WebSocket
        const ws = new WebSocket(url);
        wsRef.current = ws;

        ws.onopen = () => {
            console.log("WebSocket connected");
            term.write('\r\n\x1b[32mConnected to terminal...\x1b[0m\r\n');
        };

        ws.onmessage = (event) => {
            if (event.data instanceof Blob) {
                const reader = new FileReader();
                reader.onload = () => {
                    term.write(reader.result);
                };
                reader.readAsText(event.data);
            } else {
                term.write(event.data);
            }
        };

        ws.onclose = () => {
            // Backend sends a disconnect message, so we don't need to print another one here.
            console.log("WebSocket closed");
        };

        ws.onerror = (err) => {
            // Suppress generic "Event" errors which happen on normal close
            if (err instanceof Event && err.type === 'error') {
                console.log("WebSocket connection closed (cleanly or with generic error)");
                return;
            }
            console.error("WebSocket error:", err);
            term.write(`\r\n\x1b[31mWebSocket error: ${err}\x1b[0m\r\n`);
        };

        term.onData((data) => {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(data);
            }
        });

        // Handle resize and notify backend of new dimensions
        const sendResizeToBackend = () => {
            const dims = fitAddon.proposeDimensions();
            if (dims && ws.readyState === WebSocket.OPEN) {
                // Send resize message as JSON with special prefix
                ws.send(JSON.stringify({ type: 'resize', cols: dims.cols, rows: dims.rows }));
            }
        };

        const handleResize = () => {
            fitAddon.fit();
            sendResizeToBackend();
        };
        window.addEventListener('resize', handleResize);

        // Send initial size after connection opens
        const originalOnOpen = ws.onopen;
        ws.onopen = (event) => {
            originalOnOpen?.(event);
            setTimeout(sendResizeToBackend, 150);
        };

        // Listen for terminal resize events from xterm
        term.onResize(({ cols, rows }) => {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(JSON.stringify({ type: 'resize', cols, rows }));
            }
        });

        // Cleanup
        return () => {
            window.removeEventListener('resize', handleResize);
            if (ws.readyState === WebSocket.OPEN) {
                ws.close();
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
    }, [url]);

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
                } catch (e) {
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
