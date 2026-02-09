import React, { useEffect, useRef, useState, useCallback } from 'react';
import { TrashIcon, ArrowDownTrayIcon, ClipboardDocumentIcon, CheckIcon, ChevronRightIcon } from '@heroicons/react/24/outline';
import { useDebug, CATEGORY_COLORS, DEBUG_SOURCE } from '../../context';
import type { DebugLogEntry, DebugCategory } from '../../context';
import { SaveDebugLogs } from 'wailsjs/go/main/App';

function LogEntryRow({ entry }: { entry: DebugLogEntry }) {
    const [expanded, setExpanded] = useState(false);
    const hasDetails = entry.details !== null && entry.details !== undefined;
    const color = CATEGORY_COLORS[entry.category as DebugCategory];
    const time = new Date(entry.timestamp).toLocaleTimeString('en-US', { hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' });

    return (
        <div className="border-b border-white/5 hover:bg-white/5">
            <div
                className={`flex items-center gap-2 px-2 py-1 ${hasDetails ? 'cursor-pointer' : ''}`}
                onClick={() => hasDetails && setExpanded(!expanded)}
            >
                {hasDetails ? (
                    <ChevronRightIcon className={`h-3 w-3 text-gray-500 shrink-0 transition-transform ${expanded ? 'rotate-90' : ''}`} />
                ) : (
                    <span className="w-3 shrink-0" />
                )}
                <span className={`text-[10px] font-medium px-1 rounded shrink-0 ${
                    entry.source === DEBUG_SOURCE.BACKEND
                        ? 'bg-blue-500/20 text-blue-400'
                        : 'bg-green-500/20 text-green-400'
                }`}>
                    {entry.source === DEBUG_SOURCE.BACKEND ? 'BE' : 'FE'}
                </span>
                <span className="text-gray-500 shrink-0">{time}</span>
                <span className={`text-[10px] font-medium uppercase shrink-0 ${color?.ui || 'text-gray-400'}`}>
                    {entry.category}
                </span>
                <span className="text-gray-300 truncate">{entry.message}</span>
            </div>
            {expanded && hasDetails && (
                <div className="ml-8 mr-2 mb-1 pl-3 border-l-2 border-white/10">
                    <pre className="text-[11px] text-gray-400 whitespace-pre-wrap break-all">
                        {typeof entry.details === 'string' ? entry.details : JSON.stringify(entry.details, null, 2)}
                    </pre>
                </div>
            )}
        </div>
    );
}

export default function DebugLogViewer() {
    const { logs, clearLogs } = useDebug();
    const endRef = useRef<HTMLDivElement>(null);
    const containerRef = useRef<HTMLDivElement>(null);
    const [copied, setCopied] = useState(false);
    const [autoScroll, setAutoScroll] = useState(true);

    // Auto-scroll when new logs arrive (only if user is at bottom)
    useEffect(() => {
        if (autoScroll && containerRef.current) {
            containerRef.current.scrollTop = 0; // Newest first = scroll to top
        }
    }, [logs, autoScroll]);

    const handleScroll = useCallback(() => {
        if (containerRef.current) {
            setAutoScroll(containerRef.current.scrollTop < 10);
        }
    }, []);

    const handleCopy = async () => {
        const json = JSON.stringify(logs, null, 2);
        await navigator.clipboard.writeText(json);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    const handleSave = async () => {
        try {
            const json = JSON.stringify(logs, null, 2);
            const filename = `kubikles-debug-${new Date().toISOString().slice(0, 10)}.json`;
            await SaveDebugLogs(json, filename);
        } catch (err) {
            console.error('Failed to save debug logs:', err);
        }
    };

    return (
        <div className="h-full w-full bg-background p-4 flex flex-col">
            <div className="flex justify-between items-center mb-2 border-b border-white/10 pb-2">
                <span className="text-xs font-bold text-gray-400">Debug Logs ({logs.length})</span>
                <div className="flex gap-2">
                    <button
                        onClick={clearLogs}
                        className="flex items-center gap-1 text-xs bg-red-500/10 text-red-400 px-2 py-1 rounded hover:bg-red-500/20 transition-colors"
                        title="Clear Logs"
                    >
                        <TrashIcon className="h-3 w-3" />
                        Clear
                    </button>
                    <button
                        onClick={handleCopy}
                        className={`flex items-center gap-1 text-xs px-2 py-1 rounded transition-colors ${
                            copied
                                ? 'bg-green-500/20 text-green-400'
                                : 'bg-primary/10 text-primary hover:bg-primary/20'
                        }`}
                        title="Copy as JSON"
                    >
                        {copied ? <CheckIcon className="h-3 w-3" /> : <ClipboardDocumentIcon className="h-3 w-3" />}
                        {copied ? 'Copied!' : 'Copy'}
                    </button>
                    <button
                        onClick={handleSave}
                        className="flex items-center gap-1 text-xs bg-primary/10 text-primary px-2 py-1 rounded hover:bg-primary/20 transition-colors"
                        title="Save as JSON"
                    >
                        <ArrowDownTrayIcon className="h-3 w-3" />
                        Save
                    </button>
                </div>
            </div>
            <div
                ref={containerRef}
                onScroll={handleScroll}
                className="flex-1 overflow-y-auto font-mono text-xs text-gray-300"
            >
                {logs.length === 0 ? (
                    <div className="text-gray-500 italic p-2">No logs yet...</div>
                ) : (
                    logs.map((entry) => (
                        <LogEntryRow key={entry.id} entry={entry} />
                    ))
                )}
                <div ref={endRef} />
            </div>
        </div>
    );
}
