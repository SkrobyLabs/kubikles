import React, { useState, useRef, useEffect, useCallback } from 'react';
import { ClipboardDocumentIcon, CheckIcon } from '@heroicons/react/24/outline';
import SimpleMarkdown from './AIMarkdown';
import ToolCallCard from './ToolCallCard';
import Tooltip from '../shared/Tooltip';
import { MessageBlock } from '../../utils/aiChatBlocks';

// CostBreakdown renders a multi-line token/cost breakdown for the cost-footer tooltip.
// Cache read/write rows are omitted when their token count is 0; $ amounts are shown
// only when the matching cost field is present and the total cost is > 0 (the Codex
// provider reports tokens but no cost, yielding token-only rows).
function CostBreakdown({ usage }: { usage: any }) {
    const hasCost = (usage?.costUSD ?? 0) > 0;
    const input = usage?.inputTokens ?? 0;
    const output = usage?.outputTokens ?? 0;
    const cacheRead = usage?.cacheReadTokens ?? 0;
    const cacheWrite = usage?.cacheCreationTokens ?? 0;
    const total = input + output + cacheRead + cacheWrite;

    const rows: { label: string; tokens: number; cost?: number }[] = [
        { label: 'Input', tokens: input, cost: usage?.inputCostUSD },
        { label: 'Output', tokens: output, cost: usage?.outputCostUSD },
    ];
    if (cacheRead > 0) rows.push({ label: 'Cache read', tokens: cacheRead, cost: usage?.cacheReadCostUSD });
    if (cacheWrite > 0) rows.push({ label: 'Cache write', tokens: cacheWrite, cost: usage?.cacheWriteCostUSD });

    return (
        <div className="whitespace-normal text-[10px] text-gray-300 min-w-[9rem]">
            {rows.map((r) => (
                <div key={r.label} className="flex justify-between gap-3">
                    <span className="text-gray-400">{r.label}</span>
                    <span className="tabular-nums">
                        {r.tokens.toLocaleString()}
                        {hasCost && r.cost != null && ` · $${r.cost.toFixed(4)}`}
                    </span>
                </div>
            ))}
            <div className="flex justify-between gap-3 mt-1 pt-1 border-t border-white/10 font-medium text-gray-200">
                <span>Total</span>
                <span className="tabular-nums">
                    {total.toLocaleString()}
                    {hasCost && ` · $${usage.costUSD.toFixed(4)}`}
                </span>
            </div>
        </div>
    );
}

// Shared thinking indicator with elapsed timer (shows after 5s)
export function ThinkingBubble() {
    const [elapsed, setElapsed] = useState(0);
    const startRef = useRef(Date.now());

    useEffect(() => {
        const interval = setInterval(() => {
            setElapsed(Math.floor((Date.now() - startRef.current) / 1000));
        }, 1000);
        return () => clearInterval(interval);
    }, []);

    return (
        <div className="flex justify-start">
            <div className="bg-white/5 rounded-lg px-3 py-2 flex items-center gap-1.5 text-xs text-gray-400">
                <span className="w-1.5 h-1.5 rounded-full bg-gray-400 animate-bounce" style={{ animationDelay: '0ms' }} />
                <span className="w-1.5 h-1.5 rounded-full bg-gray-400 animate-bounce" style={{ animationDelay: '150ms' }} />
                <span className="w-1.5 h-1.5 rounded-full bg-gray-400 animate-bounce" style={{ animationDelay: '300ms' }} />
                <span className="ml-0.5">Thinking&hellip;</span>
                {elapsed >= 5 && <span className="text-gray-500">{elapsed}s</span>}
            </div>
        </div>
    );
}

// Inline animated dots shown at the end of the assistant bubble while stalled.
function InlineDots() {
    return (
        <span className="inline-flex items-center gap-1 align-middle ml-0.5">
            <span className="w-1.5 h-1.5 rounded-full bg-gray-400 animate-bounce" style={{ animationDelay: '0ms' }} />
            <span className="w-1.5 h-1.5 rounded-full bg-gray-400 animate-bounce" style={{ animationDelay: '150ms' }} />
            <span className="w-1.5 h-1.5 rounded-full bg-gray-400 animate-bounce" style={{ animationDelay: '300ms' }} />
        </span>
    );
}

export default function MessageBubble({ msg }: { msg: any }) {
    const [copied, setCopied] = useState(false);
    const [stalled, setStalled] = useState(false);
    const contentLenRef = useRef(msg.content?.length || 0);

    // Detect streaming pauses (e.g., during tool calls) to swap the caret for dots.
    useEffect(() => {
        if (!msg.streaming) {
            setStalled(false);
            return;
        }

        const currentLen = msg.content?.length || 0;
        if (currentLen !== contentLenRef.current) {
            contentLenRef.current = currentLen;
            setStalled(false);
        }

        const timer = setTimeout(() => {
            setStalled(true);
        }, 500);

        return () => clearTimeout(timer);
    }, [msg.streaming, msg.content]);

    const handleCopy = useCallback(() => {
        navigator.clipboard.writeText(msg.content).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        });
    }, [msg.content]);

    const isAssistant = msg.role === 'assistant';
    const blocks: MessageBlock[] | undefined = msg.blocks;
    const lastBlock = blocks && blocks.length > 0 ? blocks[blocks.length - 1] : undefined;
    // A running tool card renders its own spinner, so no extra indicator is needed.
    const lastBlockIsRunningTool = lastBlock?.type === 'tool_call' && lastBlock.status === 'running';

    return (
        <div className={`group flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
            <div
                className={`relative max-w-[90%] rounded-lg px-3 py-2 text-sm ${
                    msg.role === 'user'
                        ? 'bg-[color-mix(in_srgb,var(--color-primary)_15%,transparent)] text-text'
                        : msg.isError
                            ? 'bg-red-500/10 text-red-300'
                            : 'bg-white/5 text-text w-fit'
                }`}
            >
                {isAssistant ? (
                    <div className="break-words">
                        {blocks && blocks.length > 0
                            ? blocks.map((block, i) =>
                                block.type === 'text'
                                    ? <SimpleMarkdown key={i} text={block.content} />
                                    : <ToolCallCard key={block.toolId || i} block={block} />
                            )
                            : <SimpleMarkdown text={msg.content} />
                        }
                        {msg.streaming && !lastBlockIsRunningTool && (
                            stalled
                                ? <InlineDots />
                                : <span className="inline-block w-1.5 h-3.5 bg-primary ml-0.5 animate-pulse align-middle" />
                        )}
                        {!msg.streaming && msg.usage?.outputTokens > 0 && (
                            <div className="mt-2 pt-1.5 border-t border-white/5 text-[10px] text-gray-500">
                                <Tooltip position="top" content={<CostBreakdown usage={msg.usage} />}>
                                    <span className="cursor-help">
                                        {msg.usage.outputTokens} tokens
                                        {msg.usage.costUSD > 0 && ` · $${msg.usage.costUSD.toFixed(4)}`}
                                    </span>
                                </Tooltip>
                            </div>
                        )}
                    </div>
                ) : (
                    <pre className="whitespace-pre-wrap font-mono text-xs leading-relaxed break-words">{msg.content}</pre>
                )}
                {isAssistant && !msg.streaming && msg.content && (
                    <button
                        onClick={handleCopy}
                        className="absolute top-1.5 right-1.5 p-1 rounded bg-white/5 opacity-0 group-hover:opacity-100 transition-opacity text-gray-400 hover:text-text"
                        title="Copy"
                    >
                        {copied
                            ? <CheckIcon className="h-3.5 w-3.5 text-green-400" />
                            : <ClipboardDocumentIcon className="h-3.5 w-3.5" />
                        }
                    </button>
                )}
            </div>
        </div>
    );
}
