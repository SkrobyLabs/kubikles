import React, { useState, useRef, useEffect, useCallback } from 'react';
import { XMarkIcon, PlusIcon, StopIcon, ClipboardDocumentIcon, CheckIcon, ClockIcon, TrashIcon } from '@heroicons/react/24/outline';
import { useAIChat } from '../../context';
import { useK8s } from '../../context';
import { useUI } from '../../context';
import { useConfig } from '../../context';
import { navLinkRegex } from './navUtils';
import { executeNavLink } from './AIMarkdown';
import MessageBubble, { ThinkingBubble } from './MessageBubble';

const MIN_WIDTH = 280;
const MAX_WIDTH = 800;
const WIDTH_STORAGE_KEY = 'kubikles-ai-panel-width';

export default function AIPanel() {
    const {
        messages,
        sendMessage,
        isStreaming,
        cancelRequest,
        startNewChat,
        loadConversation,
        deleteConversation,
        conversationHistory,
        conversationId,
        togglePanel,
        providerAvailable,
        providerStatus,
        providerName,
        autoExecutedNavRef
    } = useAIChat();

    const { currentContext, selectedNamespaces } = useK8s();
    const { activeView, bottomTabs, activeTabId, setActiveView, navigateWithSearch, openTab, closeTab } = useUI();
    const { getConfig } = useConfig();

    const [input, setInput] = useState('');
    const [copiedAll, setCopiedAll] = useState(false);
    const [showHistory, setShowHistory] = useState(false);
    const messagesEndRef = useRef(null);
    const textareaRef = useRef(null);
    const historyRef = useRef(null);

    const defaultWidth = getConfig('ai.panelWidth') || 384;
    const [width, setWidth] = useState(() => {
        const saved = localStorage.getItem(WIDTH_STORAGE_KEY);
        return saved ? Math.min(Math.max(parseInt(saved, 10), MIN_WIDTH), MAX_WIDTH) : defaultWidth;
    });
    const isDragging = useRef(false);

    // Auto-execute nav links when an assistant message finishes streaming
    useEffect(() => {
        const lastMsg = messages[messages.length - 1];
        if (!lastMsg || lastMsg.role !== 'assistant' || lastMsg.streaming || lastMsg.isError) return;
        if (autoExecutedNavRef.current.has(lastMsg.id)) return;

        // Extract nav:// links from message content
        const links = [];
        let m;
        navLinkRegex.lastIndex = 0;
        while ((m = navLinkRegex.exec(lastMsg.content)) !== null) {
            links.push(m[2]);
        }
        if (links.length === 0) return;

        autoExecutedNavRef.current.add(lastMsg.id);
        const ctx = { setActiveView, navigateWithSearch, openTab, closeTab, currentContext };
        for (const href of links) {
            // Skip nav://yaml auto-execution — mutable editors require explicit user click
            const action = href.slice('nav://'.length).split('/')[0].split('?')[0];
            if (action === 'yaml') continue;
            executeNavLink(href, ctx);
        }
    }, [messages, setActiveView, navigateWithSearch, openTab, closeTab, currentContext, autoExecutedNavRef]);

    // Persist width
    useEffect(() => {
        localStorage.setItem(WIDTH_STORAGE_KEY, width.toString());
    }, [width]);

    // Drag resize handlers
    const handleDragStart = useCallback((e) => {
        e.preventDefault();
        isDragging.current = true;
        document.body.style.userSelect = 'none';
        document.body.style.cursor = 'col-resize';

        const onMouseMove = (e) => {
            if (!isDragging.current) return;
            const newWidth = window.innerWidth - e.clientX;
            setWidth(Math.min(Math.max(newWidth, MIN_WIDTH), MAX_WIDTH));
        };

        const onMouseUp = () => {
            isDragging.current = false;
            document.body.style.userSelect = '';
            document.body.style.cursor = '';
            document.removeEventListener('mousemove', onMouseMove);
            document.removeEventListener('mouseup', onMouseUp);
        };

        document.addEventListener('mousemove', onMouseMove);
        document.addEventListener('mouseup', onMouseUp);
    }, []);

    // Auto-scroll to bottom on new messages
    useEffect(() => {
        messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [messages]);

    // Focus textarea when panel opens
    useEffect(() => {
        textareaRef.current?.focus();
    }, []);

    // Close history dropdown on click outside
    useEffect(() => {
        if (!showHistory) return;
        const handleClickOutside = (e) => {
            if (historyRef.current && !historyRef.current.contains(e.target)) {
                setShowHistory(false);
            }
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, [showHistory]);

    const handleSend = useCallback(() => {
        if (!input.trim() || isStreaming) return;
        sendMessage(input);
        setInput('');
        // Reset textarea height
        if (textareaRef.current) {
            textareaRef.current.style.height = 'auto';
        }
    }, [input, isStreaming, sendMessage]);

    const handleKeyDown = useCallback((e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            handleSend();
        }
    }, [handleSend]);

    // Auto-resize textarea
    const handleInput = useCallback((e) => {
        setInput(e.target.value);
        const el = e.target;
        el.style.height = 'auto';
        el.style.height = Math.min(el.scrollHeight, 120) + 'px';
    }, []);

    const handleCopyConversation = useCallback(() => {
        if (messages.length === 0) return;
        const text = messages.map(m => {
            if (m.role === 'thought') return `_${m.content}_`;
            const label = m.role === 'user' ? '## Me' : '## AI';
            return `${label}\n\n${m.content}`;
        }).join('\n\n---\n\n');
        navigator.clipboard.writeText(text).then(() => {
            setCopiedAll(true);
            setTimeout(() => setCopiedAll(false), 1500);
        });
    }, [messages]);

    // Build context status line
    const activeTab = bottomTabs.find(t => t.id === activeTabId);
    const meta = activeTab?.resourceMeta;
    const contextLine = [
        currentContext || 'no context',
        meta?.namespace || (selectedNamespaces?.length > 0 ? selectedNamespaces.join(', ') : null),
        activeView,
        meta ? `${meta.kind}/${meta.name}` : (activeTab ? activeTab.title : null)
    ].filter(Boolean).join(' / ');

    if (providerAvailable === false) {
        return (
            <div className="flex h-full" style={{ width, minWidth: MIN_WIDTH }}>
                {/* Drag handle */}
                <div
                    className="w-1 cursor-col-resize bg-border hover:bg-primary transition-colors shrink-0"
                    onMouseDown={handleDragStart}
                />
                <div className="flex-1 flex flex-col bg-surface h-full overflow-hidden">
                    {/* Header */}
                    <div className="flex items-center justify-between px-3 py-2 border-b border-border">
                        <span className="text-sm font-medium text-text">AI Assistant</span>
                        <button
                            onClick={togglePanel}
                            className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-text"
                        >
                            <XMarkIcon className="h-4 w-4" />
                        </button>
                    </div>
                    {/* Provider unavailable */}
                    <div className="flex-1 flex items-center justify-center p-4">
                        <div className="text-center text-sm text-gray-400 space-y-2">
                            <p>{providerStatus || 'AI provider is not available.'}</p>
                        </div>
                    </div>
                </div>
            </div>
        );
    }

    return (
        <div className="flex h-full" style={{ width, minWidth: MIN_WIDTH }}>
            {/* Drag handle */}
            <div
                className="w-1 cursor-col-resize bg-border hover:bg-primary transition-colors shrink-0"
                onMouseDown={handleDragStart}
            />
            <div className="flex-1 flex flex-col bg-surface h-full overflow-hidden">
            {/* Header */}
            <div className="flex items-center justify-between px-3 py-2 border-b border-border">
                <div className="flex flex-col">
                    <span className="text-sm font-medium text-text">AI Assistant</span>
                    {providerName && (
                        <span className="text-[10px] text-gray-500">
                            {providerName}{getConfig('ai.model') ? ` · ${getConfig('ai.model')}` : ''}
                        </span>
                    )}
                </div>
                <div className="flex items-center gap-1">
                    {messages.length > 0 && (
                        <button
                            onClick={handleCopyConversation}
                            className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-text"
                            title="Copy conversation"
                        >
                            {copiedAll
                                ? <CheckIcon className="h-4 w-4 text-green-400" />
                                : <ClipboardDocumentIcon className="h-4 w-4" />
                            }
                        </button>
                    )}
                    <button
                        onClick={startNewChat}
                        className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-text"
                        title="New chat"
                    >
                        <PlusIcon className="h-4 w-4" />
                    </button>
                    <div className="relative" ref={historyRef}>
                        <button
                            onClick={() => setShowHistory(prev => !prev)}
                            className={`p-1 rounded hover:bg-white/10 text-gray-400 hover:text-text ${showHistory ? 'bg-white/10 text-text' : ''}`}
                            title="Chat history"
                        >
                            <ClockIcon className="h-4 w-4" />
                        </button>
                        {showHistory && (
                            <div className="absolute right-0 top-full mt-1 w-64 bg-surface border border-border rounded-lg shadow-lg z-50 overflow-hidden">
                                <div className="px-3 py-2 border-b border-border text-xs font-medium text-gray-400">
                                    Recent conversations
                                </div>
                                <div className="max-h-64 overflow-y-auto">
                                    {conversationHistory.length === 0 ? (
                                        <div className="px-3 py-4 text-xs text-gray-500 text-center">
                                            No conversation history
                                        </div>
                                    ) : (
                                        conversationHistory.map(conv => (
                                            <div
                                                key={conv.id}
                                                className={`group flex items-center gap-2 px-3 py-2 hover:bg-white/5 cursor-pointer ${conv.id === conversationId ? 'bg-white/10' : ''}`}
                                            >
                                                <button
                                                    onClick={() => {
                                                        loadConversation(conv.id);
                                                        setShowHistory(false);
                                                    }}
                                                    className="flex-1 text-left min-w-0"
                                                >
                                                    <div className="text-xs text-text truncate">{conv.title}</div>
                                                    <div className="text-[10px] text-gray-500">
                                                        {new Date(conv.updatedAt).toLocaleDateString()} · {conv.messages.length} msgs
                                                    </div>
                                                </button>
                                                <button
                                                    onClick={(e) => {
                                                        e.stopPropagation();
                                                        deleteConversation(conv.id);
                                                    }}
                                                    className="p-1 rounded hover:bg-white/10 text-gray-500 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity"
                                                    title="Delete conversation"
                                                >
                                                    <TrashIcon className="h-3.5 w-3.5" />
                                                </button>
                                            </div>
                                        ))
                                    )}
                                </div>
                            </div>
                        )}
                    </div>
                    <button
                        onClick={togglePanel}
                        className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-text"
                        title="Close"
                    >
                        <XMarkIcon className="h-4 w-4" />
                    </button>
                </div>
            </div>

            {/* Messages */}
            <div className="flex-1 overflow-y-auto px-3 py-2 space-y-3">
                {messages.length === 0 && (
                    <div className="text-center text-sm text-gray-500 mt-8">
                        <p>Ask anything about Kubernetes.</p>
                        <p className="text-xs text-gray-600 mt-1">
                            Context is automatically included.
                        </p>
                    </div>
                )}
                {messages.map((msg) => (
                    <MessageBubble key={msg.id} msg={msg} />
                ))}
                {isStreaming && !messages.some(m => m.streaming) && <ThinkingBubble />}
                <div ref={messagesEndRef} />
            </div>

            {/* Input */}
            <div className="border-t border-border px-3 py-2 space-y-1">
                <div className="flex items-end gap-2">
                    <textarea
                        ref={textareaRef}
                        value={input}
                        onChange={handleInput}
                        onKeyDown={handleKeyDown}
                        placeholder="Ask about Kubernetes..."
                        rows={1}
                        className="flex-1 bg-background border border-border rounded px-2 py-1.5 text-sm text-text placeholder-gray-500 outline-none focus:border-primary resize-none overflow-hidden"
                        disabled={isStreaming}
                        autoComplete="off"
                        autoCorrect="off"
                        autoCapitalize="off"
                        spellCheck={false}
                    />
                    {isStreaming ? (
                        <button
                            onClick={cancelRequest}
                            className="px-2.5 py-1.5 bg-red-600/20 hover:bg-red-600/30 text-red-400 rounded text-sm shrink-0"
                            title="Stop"
                        >
                            <StopIcon className="h-4 w-4" />
                        </button>
                    ) : (
                        <button
                            onClick={handleSend}
                            disabled={!input.trim()}
                            className="px-2.5 py-1.5 bg-primary/20 hover:bg-primary/30 text-primary rounded text-sm disabled:opacity-30 disabled:cursor-not-allowed shrink-0"
                        >
                            Send
                        </button>
                    )}
                </div>
                <div className="text-[10px] text-gray-600 truncate">
                    {contextLine}
                </div>
            </div>
            </div>
        </div>
    );
}
