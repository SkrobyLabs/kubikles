import React, { useEffect, useRef } from 'react';
import { Terminal as XTerm } from 'xterm';
import { FitAddon } from 'xterm-addon-fit';
import 'xterm/css/xterm.css';

const Terminal = ({ url }) => {
    const terminalRef = useRef(null);
    const xtermRef = useRef(null);
    const wsRef = useRef(null);
    const fitAddonRef = useRef(null);

    useEffect(() => {
        // Initialize xterm
        const term = new XTerm({
            cursorBlink: true,
            theme: {
                background: '#1e1e1e',
                foreground: '#ffffff',
            },
            fontSize: 14,
            fontFamily: 'Menlo, Monaco, "Courier New", monospace',
        });

        const fitAddon = new FitAddon();
        term.loadAddon(fitAddon);
        fitAddonRef.current = fitAddon;

        term.open(terminalRef.current);
        fitAddon.fit();

        xtermRef.current = term;

        // Connect to WebSocket
        const ws = new WebSocket(url);
        wsRef.current = ws;

        ws.onopen = () => {
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
            term.write('\r\n\x1b[33mTerminal disconnected...\x1b[0m\r\n');
        };

        ws.onerror = (err) => {
            term.write(`\r\n\x1b[31mWebSocket error: ${err}\x1b[0m\r\n`);
        };

        term.onData((data) => {
            if (ws.readyState === WebSocket.OPEN) {
                ws.send(data);
            }
        });

        // Handle resize
        const handleResize = () => {
            fitAddon.fit();
        };
        window.addEventListener('resize', handleResize);

        // Cleanup
        return () => {
            window.removeEventListener('resize', handleResize);
            if (ws.readyState === WebSocket.OPEN) {
                ws.close();
            }
            term.dispose();
        };
    }, [url]);

    // Re-fit when the container size changes (e.g. split view resize)
    useEffect(() => {
        const observer = new ResizeObserver(() => {
            if (fitAddonRef.current) {
                fitAddonRef.current.fit();
            }
        });
        if (terminalRef.current) {
            observer.observe(terminalRef.current);
        }
        return () => observer.disconnect();
    }, []);

    return (
        <div
            ref={terminalRef}
            className="h-full w-full bg-[#1e1e1e]"
            style={{ padding: '10px' }}
        />
    );
};

export default Terminal;
