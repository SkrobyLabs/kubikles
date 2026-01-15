import React, { useEffect, useRef } from 'react';
import { TrashIcon, ArrowDownTrayIcon } from '@heroicons/react/24/outline';

export default function DebugLogViewer({ logs, onClear, onDownload }) {
    const endRef = useRef(null);

    useEffect(() => {
        endRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [logs]);

    return (
        <div className="h-full w-full bg-background p-4 flex flex-col">
            <div className="flex justify-between items-center mb-2 border-b border-white/10 pb-2">
                <span className="text-xs font-bold text-gray-400">Backend Logs ({logs.length})</span>
                <div className="flex gap-2">
                    <button
                        onClick={onClear}
                        className="flex items-center gap-1 text-xs bg-red-500/10 text-red-400 px-2 py-1 rounded hover:bg-red-500/20 transition-colors"
                        title="Clear Logs"
                    >
                        <TrashIcon className="h-3 w-3" />
                        Clear
                    </button>
                    <button
                        onClick={onDownload}
                        className="flex items-center gap-1 text-xs bg-primary/10 text-primary px-2 py-1 rounded hover:bg-primary/20 transition-colors"
                        title="Download Logs"
                    >
                        <ArrowDownTrayIcon className="h-3 w-3" />
                        Download
                    </button>
                </div>
            </div>
            <div className="flex-1 overflow-y-auto font-mono text-xs text-gray-300 whitespace-pre-wrap">
                {logs.length === 0 ? (
                    <div className="text-gray-500 italic p-2">No logs yet...</div>
                ) : (
                    logs.map((log, i) => (
                        <div key={i} className="border-b border-white/5 py-1 hover:bg-white/5 px-1">
                            {log}
                        </div>
                    ))
                )}
                <div ref={endRef} />
            </div>
        </div>
    );
}
