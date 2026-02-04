import React, { useState, useRef, useCallback } from 'react';
import { createPortal } from 'react-dom';
import Editor from '@monaco-editor/react';
import { XMarkIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { ApplyYAML } from '../../../wailsjs/go/main/App';
import { useNotification } from '../../context';

/**
 * CreateResourceModal - Modal for creating new Kubernetes resources from YAML
 *
 * @param {Object} props
 * @param {boolean} props.isOpen - Whether the modal is open
 * @param {Function} props.onClose - Called when modal is closed
 * @param {Function} props.onSuccess - Called after successful creation (for refresh)
 * @param {string} props.title - Modal title (e.g., "Create Pod")
 * @param {string} props.template - Initial YAML template
 */
export default function CreateResourceModal({
    isOpen,
    onClose,
    onSuccess,
    title = 'Create Resource',
    template = '',
}) {
    const { addNotification } = useNotification();
    const [content, setContent] = useState(template);
    const [error, setError] = useState(null);
    const [creating, setCreating] = useState(false);
    const editorRef = useRef(null);

    // Reset state when modal opens with new template
    React.useEffect(() => {
        if (isOpen) {
            setContent(template);
            setError(null);
            setCreating(false);
        }
    }, [isOpen, template]);

    const handleEditorDidMount = useCallback((editor, monaco) => {
        editorRef.current = editor;

        editor.updateOptions({
            minimap: { enabled: false },
            scrollBeyondLastLine: false,
            fontSize: 13,
            lineNumbers: 'on',
            folding: true,
            renderWhitespace: 'selection',
            wordWrap: 'off',
        });

        // Focus the editor
        setTimeout(() => editor.focus(), 100);

        // Add Cmd+Enter / Ctrl+Enter to submit
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.Enter, () => {
            handleCreate();
        });
    }, []);

    const handleCreate = async () => {
        if (creating) return;

        setCreating(true);
        setError(null);

        try {
            await ApplyYAML(content);
            addNotification({
                type: 'success',
                title: 'Resource created',
                message: 'The resource was created successfully',
            });
            onSuccess?.();
            onClose();
        } catch (err) {
            const errMessage = err?.message || String(err);
            setError(errMessage);
        } finally {
            setCreating(false);
        }
    };

    const handleBackdropClick = useCallback((e) => {
        if (e.target === e.currentTarget && !creating) {
            onClose();
        }
    }, [creating, onClose]);

    const handleKeyDown = useCallback((e) => {
        if (e.key === 'Escape' && !creating) {
            onClose();
        }
    }, [creating, onClose]);

    React.useEffect(() => {
        if (isOpen) {
            window.addEventListener('keydown', handleKeyDown);
            return () => window.removeEventListener('keydown', handleKeyDown);
        }
    }, [isOpen, handleKeyDown]);

    if (!isOpen) return null;

    return createPortal(
        <div
            className="fixed inset-0 bg-black/60 flex items-center justify-center z-50"
            onClick={handleBackdropClick}
        >
            <div
                className="bg-surface border border-border rounded-lg shadow-2xl w-full max-w-3xl h-[70vh] flex flex-col overflow-hidden"
                onClick={(e) => e.stopPropagation()}
            >
                {/* Header */}
                <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
                    <h2 className="text-lg font-semibold text-text">{title}</h2>
                    <button
                        onClick={onClose}
                        disabled={creating}
                        className="p-1 hover:bg-white/10 rounded transition-colors disabled:opacity-50"
                    >
                        <XMarkIcon className="h-5 w-5 text-gray-400" />
                    </button>
                </div>

                {/* Error Banner */}
                {error && (
                    <div className="flex items-start gap-2 px-4 py-3 bg-red-500/20 border-b border-red-500/50 text-red-400 shrink-0">
                        <ExclamationTriangleIcon className="h-5 w-5 shrink-0 mt-0.5" />
                        <div className="text-sm flex-1">
                            <div className="font-medium">Failed to create resource</div>
                            <div className="text-red-300/80 mt-1 whitespace-pre-wrap break-words">{error}</div>
                        </div>
                    </div>
                )}

                {/* Editor */}
                <div className="flex-1 overflow-hidden">
                    <Editor
                        height="100%"
                        defaultLanguage="yaml"
                        value={content}
                        onChange={(value) => setContent(value || '')}
                        onMount={handleEditorDidMount}
                        theme="vs-dark"
                        options={{
                            automaticLayout: true,
                            scrollbar: {
                                vertical: 'auto',
                                horizontal: 'auto',
                            },
                        }}
                    />
                </div>

                {/* Footer */}
                <div className="flex items-center justify-between px-4 py-3 border-t border-border shrink-0">
                    <div className="text-xs text-gray-500">
                        <kbd className="px-1.5 py-0.5 bg-background rounded border border-border">Cmd+Enter</kbd> to create
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            onClick={onClose}
                            disabled={creating}
                            className="px-4 py-1.5 text-sm text-gray-300 hover:text-text hover:bg-white/10 rounded transition-colors disabled:opacity-50"
                        >
                            Cancel
                        </button>
                        <button
                            onClick={handleCreate}
                            disabled={creating}
                            className="px-4 py-1.5 text-sm bg-primary hover:bg-primary/90 text-white rounded transition-colors disabled:opacity-50"
                        >
                            {creating ? 'Creating...' : 'Create'}
                        </button>
                    </div>
                </div>
            </div>
        </div>,
        document.body
    );
}
