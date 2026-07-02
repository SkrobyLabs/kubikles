import React, { useState } from 'react';
import { ChevronRightIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { ToolCallBlock } from '../../utils/aiChatBlocks';

// Strip the MCP prefix so cards show a friendly tool name.
function shortToolName(name: string): string {
    const idx = name.lastIndexOf('__');
    return idx >= 0 ? name.slice(idx + 2) : name;
}

// Build a one-line summary of the tool input JSON values.
function inputSummary(input?: string): string {
    if (!input) return '';
    try {
        const parsed = JSON.parse(input);
        if (parsed && typeof parsed === 'object') {
            const summary = Object.values(parsed)
                .map(v => (typeof v === 'string' ? v : JSON.stringify(v)))
                .join(', ');
            return summary.length > 60 ? summary.slice(0, 60) + '…' : summary;
        }
    } catch {
        // fall through to raw truncation
    }
    return input.length > 60 ? input.slice(0, 60) + '…' : input;
}

export default function ToolCallCard({ block }: { block: ToolCallBlock }) {
    const [expanded, setExpanded] = useState(false);
    const isError = block.status === 'error';
    const isRunning = block.status === 'running';
    const summary = inputSummary(block.input);

    return (
        <div className={`my-1.5 rounded border text-xs ${isError ? 'border-red-500/40 bg-red-500/10' : 'border-border bg-black/20'}`}>
            <button
                onClick={() => setExpanded(e => !e)}
                className="flex w-full items-center gap-1.5 px-2 py-1.5 text-left text-gray-300 hover:text-text"
            >
                <ChevronRightIcon className={`h-3 w-3 flex-shrink-0 transition-transform ${expanded ? 'rotate-90' : ''}`} />
                {isRunning ? (
                    <span className="h-3 w-3 flex-shrink-0 rounded-full border-2 border-gray-500 border-t-primary animate-spin" />
                ) : isError ? (
                    <ExclamationTriangleIcon className="h-3 w-3 flex-shrink-0 text-red-400" />
                ) : null}
                <span className="font-mono font-medium flex-shrink-0">{shortToolName(block.name)}</span>
                {summary && <span className="truncate text-gray-500">{summary}</span>}
            </button>
            {expanded && (
                <div className="border-t border-white/5 px-2 py-1.5 space-y-1.5">
                    {block.input && (
                        <pre className="whitespace-pre-wrap font-mono text-[11px] text-gray-400">{block.input}</pre>
                    )}
                    {block.result !== undefined && (
                        <pre className="max-h-48 overflow-y-auto whitespace-pre-wrap font-mono text-[11px] text-gray-300">{block.result}</pre>
                    )}
                </div>
            )}
        </div>
    );
}
