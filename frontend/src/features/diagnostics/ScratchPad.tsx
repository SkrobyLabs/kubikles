import React, { useState, useRef, useCallback } from 'react';
import Editor from '@monaco-editor/react';
// @ts-ignore - no declaration file for js-yaml
import yaml from 'js-yaml';
import { useNotification } from '~/context';
import {
    PencilSquareIcon,
    SparklesIcon,
    ClipboardDocumentIcon,
    TrashIcon,
    XMarkIcon,
} from '@heroicons/react/24/outline';

// Scratch pad content/language persist across tab switches and app restarts.
const CONTENT_KEY = 'kubikles.scratchpad.content';
const LANGUAGE_KEY = 'kubikles.scratchpad.language';

// Languages offered for syntax highlighting. Those flagged `formattable`
// get an active "Format" button (pretty-print); others are highlight-only.
const LANGUAGES: { value: string; label: string; formattable?: boolean }[] = [
    { value: 'plaintext', label: 'Plain Text' },
    { value: 'json', label: 'JSON', formattable: true },
    { value: 'yaml', label: 'YAML', formattable: true },
    { value: 'xml', label: 'XML' },
    { value: 'sql', label: 'SQL' },
    { value: 'shell', label: 'Shell' },
    { value: 'markdown', label: 'Markdown' },
    { value: 'javascript', label: 'JavaScript' },
    { value: 'go', label: 'Go' },
];

const loadStored = (key: string, fallback: string): string => {
    try {
        return localStorage.getItem(key) ?? fallback;
    } catch {
        return fallback;
    }
};

const store = (key: string, value: string) => {
    try {
        localStorage.setItem(key, value);
    } catch {
        // localStorage may be unavailable (private mode); scratchpad still works in-session.
    }
};

export default function ScratchPad({ onClose }: { onClose?: () => void }) {
    const { addNotification } = useNotification();
    const [content, setContent] = useState(() => loadStored(CONTENT_KEY, ''));
    const [language, setLanguage] = useState(() => loadStored(LANGUAGE_KEY, 'plaintext'));
    const editorRef = useRef<any>(null);

    const formattable = LANGUAGES.find(l => l.value === language)?.formattable;

    const updateContent = useCallback((value: string) => {
        setContent(value);
        store(CONTENT_KEY, value);
    }, []);

    const handleLanguageChange = (value: string) => {
        setLanguage(value);
        store(LANGUAGE_KEY, value);
    };

    const handleFormat = () => {
        if (!content.trim()) return;
        try {
            let formatted = content;
            if (language === 'json') {
                formatted = JSON.stringify(JSON.parse(content), null, 2);
            } else if (language === 'yaml') {
                formatted = yaml.dump(yaml.load(content), { indent: 2, lineWidth: -1 });
            }
            updateContent(formatted);
        } catch (err) {
            addNotification({
                type: 'error',
                title: `Invalid ${language.toUpperCase()}`,
                message: err instanceof Error ? err.message : String(err),
            });
        }
    };

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(content);
            addNotification({ type: 'success', title: 'Copied to clipboard', message: '' });
        } catch (err) {
            addNotification({ type: 'error', title: 'Copy failed', message: String(err) });
        }
    };

    const handleClear = () => {
        updateContent('');
        editorRef.current?.focus();
    };

    return (
        <div className="h-full flex flex-col bg-background text-text">
            {/* Header (draggable window region; buttons opt out via global no-drag CSS) */}
            <div className="flex-shrink-0 border-b border-border px-4 h-14 flex items-center justify-between gap-4 titlebar-drag">
                <h2 className="text-lg font-semibold flex items-center gap-2 shrink-0">
                    <PencilSquareIcon className="h-5 w-5 text-amber-400" />
                    Scratch Pad
                </h2>
                <div className="flex items-center gap-2">
                    <select
                        value={language}
                        onChange={(e) => handleLanguageChange(e.target.value)}
                        className="bg-surface border border-border rounded px-2 py-1 text-sm text-text focus:outline-none focus:border-primary"
                        title="Syntax highlighting"
                    >
                        {LANGUAGES.map(l => (
                            <option key={l.value} value={l.value}>{l.label}</option>
                        ))}
                    </select>
                    <button
                        onClick={handleFormat}
                        disabled={!formattable}
                        className="flex items-center gap-1.5 px-2.5 py-1 text-sm rounded border border-border hover:bg-white/10 transition-colors disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-transparent"
                        title={formattable ? `Pretty-print ${language.toUpperCase()}` : 'Formatting available for JSON and YAML'}
                    >
                        <SparklesIcon className="h-4 w-4" />
                        Format
                    </button>
                    <button
                        onClick={handleCopy}
                        className="flex items-center gap-1.5 px-2.5 py-1 text-sm rounded border border-border hover:bg-white/10 transition-colors"
                        title="Copy all to clipboard"
                    >
                        <ClipboardDocumentIcon className="h-4 w-4" />
                        Copy
                    </button>
                    <button
                        onClick={handleClear}
                        className="flex items-center gap-1.5 px-2.5 py-1 text-sm rounded border border-border hover:bg-white/10 transition-colors"
                        title="Clear scratch pad"
                    >
                        <TrashIcon className="h-4 w-4" />
                        Clear
                    </button>
                    {onClose && (
                        <button
                            onClick={onClose}
                            className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                            title="Close"
                        >
                            <XMarkIcon className="h-5 w-5" />
                        </button>
                    )}
                </div>
            </div>

            {/* Editor */}
            <div className="flex-1 min-h-0">
                <Editor
                    height="100%"
                    language={language}
                    value={content}
                    onChange={(value) => updateContent(value || '')}
                    onMount={(editor) => { editorRef.current = editor; }}
                    theme="vs-dark"
                    options={{
                        automaticLayout: true,
                        wordWrap: 'on',
                        minimap: { enabled: false },
                        scrollBeyondLastLine: false,
                        scrollbar: { vertical: 'auto', horizontal: 'auto' },
                    }}
                />
            </div>
        </div>
    );
}
