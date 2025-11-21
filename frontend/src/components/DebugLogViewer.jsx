import React, { useEffect, useRef } from 'react';

export default function DebugLogViewer({ logs, onTestEmit }) {
    const endRef = useRef(null);

    useEffect(() => {
        endRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [logs]);

    return (
        <div className="h-full w-full bg-[#1e1e1e] p-4 flex flex-col">
            <div className="flex justify-between items-center mb-2 border-b border-white/10 pb-2">
                <span className="text-xs font-bold text-gray-400">Backend Logs</span>
                <div className="flex gap-2">
                    <button
                        onClick={() => onTestEmit('ui')}
                        className="text-xs bg-gray-700 text-white px-2 py-1 rounded hover:bg-gray-600"
                    >
                        Test UI
                    </button>
                    <button
                        onClick={() => onTestEmit('backend')}
                        className="text-xs bg-primary/20 text-primary px-2 py-1 rounded hover:bg-primary/30"
                    >
                        Test Backend
                    </button>
                </div>
            </div>
            <div className="flex-1 overflow-y-auto font-mono text-xs text-gray-300 whitespace-pre-wrap">
                {logs.length === 0 ? "No logs yet..." : logs.map((log, i) => (
                    <div key={i} className="border-b border-white/5 py-1">{log}</div>
                ))}
                <div ref={endRef} />
            </div>
        </div>
    );
}
