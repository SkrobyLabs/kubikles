import React, { useState, useRef, useEffect, useCallback } from 'react';
import { ClipboardDocumentIcon, CheckIcon } from '@heroicons/react/24/outline';
import SimpleMarkdown from './AIMarkdown';

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

export default function MessageBubble({ msg }) {
    const [copied, setCopied] = useState(false);
    const [isThinking, setIsThinking] = useState(false);
    const contentLenRef = useRef(msg.content?.length || 0);

    // Detect streaming pauses (e.g., during tool calls) and show thinking indicator
    useEffect(() => {
        if (!msg.streaming) {
            setIsThinking(false);
            return;
        }

        const currentLen = msg.content?.length || 0;
        if (currentLen !== contentLenRef.current) {
            contentLenRef.current = currentLen;
            setIsThinking(false);
        }

        const timer = setTimeout(() => {
            setIsThinking(true);
        }, 500);

        return () => clearTimeout(timer);
    }, [msg.streaming, msg.content]);

    const handleCopy = useCallback(() => {
        navigator.clipboard.writeText(msg.content).then(() => {
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
        });
    }, [msg.content]);

    // Thought bubble — permanent record of a thinking pause
    if (msg.role === 'thought') {
        return (
            <div className="flex justify-center">
                <span className="text-[11px] text-gray-500 italic">{msg.content}</span>
            </div>
        );
    }

    const isAssistant = msg.role === 'assistant';

    return (
        <>
            <div className={`group flex ${msg.role === 'user' ? 'justify-end' : 'justify-start'}`}>
                <div
                    className={`relative max-w-[90%] rounded-lg px-3 py-2 text-sm ${
                        msg.role === 'user'
                            ? 'bg-accent/10 text-text'
                            : msg.isError
                                ? 'bg-red-500/10 text-red-300'
                                : 'bg-white/5 text-text'
                    }`}
                >
                    {isAssistant ? (
                        <div className="break-words">
                            <SimpleMarkdown text={msg.content} />
                            {msg.streaming && !isThinking && (
                                <span className="inline-block w-1.5 h-3.5 bg-primary ml-0.5 animate-pulse" />
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
            {msg.streaming && isThinking && <ThinkingBubble />}
        </>
    );
}
