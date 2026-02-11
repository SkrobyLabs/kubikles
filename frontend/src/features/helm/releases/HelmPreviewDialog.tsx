import React, { useState, useCallback, useEffect, useRef } from 'react';
import { XMarkIcon, ArrowPathIcon, ExclamationTriangleIcon, DocumentTextIcon, DocumentDuplicateIcon } from '@heroicons/react/24/outline';
import { HelmTemplateRelease, HelmDryRunUpgrade } from 'wailsjs/go/main/App';
import Editor from '@monaco-editor/react';

type PreviewMode = 'template' | 'dryrun';

interface HelmPreviewDialogProps {
    release: any;
    upgradeOpts: any; // UpgradeOptions-shaped object
    onClose: () => void;
}

export default function HelmPreviewDialog({ release, upgradeOpts, onClose }: HelmPreviewDialogProps) {
    const [mode, setMode] = useState<PreviewMode>('template');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // Template mode state
    const [templateManifest, setTemplateManifest] = useState('');
    const [templateNotes, setTemplateNotes] = useState('');

    // Dry-run mode state
    const [currentManifest, setCurrentManifest] = useState('');
    const [proposedManifest, setProposedManifest] = useState('');
    const [dryRunNotes, setDryRunNotes] = useState('');

    // Track which mode has been loaded
    const loadedRef = useRef<{ template: boolean; dryrun: boolean }>({ template: false, dryrun: false });
    const editorRef = useRef<any>(null);

    const fetchTemplate = useCallback(async () => {
        if (loadedRef.current.template) return;
        setLoading(true);
        setError(null);
        try {
            const result = await HelmTemplateRelease(release.name, release.namespace, upgradeOpts);
            setTemplateManifest(result.manifests || '');
            setTemplateNotes(result.notes || '');
            loadedRef.current.template = true;
        } catch (err: any) {
            setError(err?.message || String(err));
        } finally {
            setLoading(false);
        }
    }, [release, upgradeOpts]);

    const fetchDryRun = useCallback(async () => {
        if (loadedRef.current.dryrun) return;
        setLoading(true);
        setError(null);
        try {
            const result = await HelmDryRunUpgrade(release.namespace, release.name, upgradeOpts);
            setCurrentManifest(result.currentManifest || '');
            setProposedManifest(result.proposedManifest || '');
            setDryRunNotes(result.notes || '');
            loadedRef.current.dryrun = true;
        } catch (err: any) {
            setError(err?.message || String(err));
        } finally {
            setLoading(false);
        }
    }, [release, upgradeOpts]);

    // Load data when mode changes
    useEffect(() => {
        if (mode === 'template') {
            fetchTemplate();
        } else {
            fetchDryRun();
        }
    }, [mode, fetchTemplate, fetchDryRun]);

    const handleRetry = () => {
        // Clear cache for current mode so it re-fetches
        if (mode === 'template') {
            loadedRef.current.template = false;
            fetchTemplate();
        } else {
            loadedRef.current.dryrun = false;
            fetchDryRun();
        }
    };

    const handleCopy = async () => {
        const text = mode === 'template' ? templateManifest : proposedManifest;
        if (text) {
            await navigator.clipboard.writeText(text);
        }
    };

    // Compute a simple inline diff summary for dry-run mode
    const diffSummary = currentManifest && proposedManifest
        ? computeDiffSummary(currentManifest, proposedManifest)
        : null;

    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50">
            <div className="bg-surface border border-border rounded-lg shadow-xl w-full max-w-5xl h-[90vh] overflow-hidden flex flex-col">
                {/* Header */}
                <div className="flex items-center justify-between px-6 py-4 border-b border-border shrink-0">
                    <div className="flex items-center gap-4">
                        <div>
                            <h2 className="text-lg font-semibold">Preview Changes</h2>
                            <p className="text-sm text-gray-400 mt-0.5">
                                {release.name} &rarr; {upgradeOpts.chartName}:{upgradeOpts.version || 'latest'}
                            </p>
                        </div>
                        {/* Mode Toggle */}
                        <div className="flex items-center bg-surface-light rounded-md p-0.5 ml-4">
                            <button
                                onClick={() => setMode('template')}
                                className={`px-3 py-1.5 text-xs font-medium rounded transition-colors flex items-center gap-1.5 ${
                                    mode === 'template'
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                <DocumentTextIcon className="h-3.5 w-3.5" />
                                Template
                            </button>
                            <button
                                onClick={() => setMode('dryrun')}
                                className={`px-3 py-1.5 text-xs font-medium rounded transition-colors flex items-center gap-1.5 ${
                                    mode === 'dryrun'
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                }`}
                            >
                                <DocumentDuplicateIcon className="h-3.5 w-3.5" />
                                Dry Run Diff
                            </button>
                        </div>
                    </div>
                    <div className="flex items-center gap-2">
                        <button
                            onClick={handleCopy}
                            disabled={loading}
                            className="px-3 py-1.5 text-xs text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors disabled:opacity-50"
                            title="Copy manifest to clipboard"
                        >
                            Copy
                        </button>
                        <button
                            onClick={onClose}
                            className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                        >
                            <XMarkIcon className="h-5 w-5" />
                        </button>
                    </div>
                </div>

                {/* Info Banner */}
                {mode === 'dryrun' && !loading && !error && (
                    <div className="flex items-center gap-2 px-6 py-2 bg-yellow-500/10 border-b border-yellow-500/30 text-yellow-400 text-xs shrink-0">
                        <ExclamationTriangleIcon className="h-4 w-4 shrink-0" />
                        <span>Dry run contacts the cluster for server-side validation. No actual changes are applied.</span>
                        {diffSummary && (
                            <span className="ml-auto text-gray-400">
                                {diffSummary.added > 0 && <span className="text-green-400 mr-2">+{diffSummary.added} added</span>}
                                {diffSummary.removed > 0 && <span className="text-red-400 mr-2">-{diffSummary.removed} removed</span>}
                                {diffSummary.changed === 0 && diffSummary.added === 0 && diffSummary.removed === 0 && <span>No changes</span>}
                            </span>
                        )}
                    </div>
                )}

                {mode === 'template' && !loading && !error && (
                    <div className="flex items-center gap-2 px-6 py-2 bg-blue-500/10 border-b border-blue-500/30 text-blue-400 text-xs shrink-0">
                        <DocumentTextIcon className="h-4 w-4 shrink-0" />
                        <span>Client-side template rendering. No cluster access required.</span>
                    </div>
                )}

                {/* Content */}
                <div className="flex-1 overflow-hidden">
                    {loading ? (
                        <div className="flex items-center justify-center h-full text-gray-400">
                            <ArrowPathIcon className="h-6 w-6 animate-spin mr-3" />
                            <span>{mode === 'template' ? 'Rendering templates...' : 'Running dry-run upgrade...'}</span>
                        </div>
                    ) : error ? (
                        <div className="flex flex-col items-center justify-center h-full p-8">
                            <ExclamationTriangleIcon className="h-10 w-10 text-red-400 mb-4" />
                            <p className="text-red-400 text-center max-w-lg mb-4">{error}</p>
                            <button
                                onClick={handleRetry}
                                className="px-4 py-2 text-sm bg-primary/20 hover:bg-primary/30 text-primary rounded transition-colors"
                            >
                                Retry
                            </button>
                        </div>
                    ) : mode === 'template' ? (
                        <div className="h-full flex flex-col">
                            {templateNotes && (
                                <div className="px-4 py-2 bg-background border-b border-border text-xs text-gray-400 max-h-24 overflow-y-auto">
                                    <span className="font-medium text-gray-300">Notes: </span>
                                    <span className="whitespace-pre-wrap">{templateNotes}</span>
                                </div>
                            )}
                            <div className="flex-1">
                                <Editor
                                    height="100%"
                                    language="yaml"
                                    value={templateManifest}
                                    theme="vs-dark"
                                    onMount={(editor) => { editorRef.current = editor; }}
                                    options={{
                                        readOnly: true,
                                        minimap: { enabled: true },
                                        scrollBeyondLastLine: false,
                                        fontSize: 12,
                                        lineNumbers: 'on',
                                        renderLineHighlight: 'line',
                                        automaticLayout: true,
                                        wordWrap: 'off',
                                        folding: true,
                                        foldingStrategy: 'indentation',
                                    }}
                                />
                            </div>
                        </div>
                    ) : (
                        <div className="h-full flex flex-col">
                            {dryRunNotes && (
                                <div className="px-4 py-2 bg-background border-b border-border text-xs text-gray-400 max-h-24 overflow-y-auto">
                                    <span className="font-medium text-gray-300">Notes: </span>
                                    <span className="whitespace-pre-wrap">{dryRunNotes}</span>
                                </div>
                            )}
                            <DiffView original={currentManifest} modified={proposedManifest} />
                        </div>
                    )}
                </div>

                {/* Footer */}
                <div className="flex justify-end px-6 py-3 border-t border-border shrink-0">
                    <button
                        onClick={onClose}
                        className="px-4 py-2 text-sm text-gray-400 hover:text-white hover:bg-white/10 rounded-md transition-colors"
                    >
                        Close
                    </button>
                </div>
            </div>
        </div>
    );
}

// DiffView renders a side-by-side diff using two Monaco editors with synchronized scrolling
function DiffView({ original, modified }: { original: string; modified: string }) {
    const leftEditorRef = useRef<any>(null);
    const rightEditorRef = useRef<any>(null);
    const isSyncing = useRef(false);

    // Synchronized scrolling between the two editors
    const syncScroll = useCallback((source: 'left' | 'right') => {
        if (isSyncing.current) return;
        isSyncing.current = true;

        const sourceEditor = source === 'left' ? leftEditorRef.current : rightEditorRef.current;
        const targetEditor = source === 'left' ? rightEditorRef.current : leftEditorRef.current;

        if (sourceEditor && targetEditor) {
            const scrollTop = sourceEditor.getScrollTop();
            targetEditor.setScrollTop(scrollTop);
        }

        requestAnimationFrame(() => { isSyncing.current = false; });
    }, []);

    // Compute inline diff decorations
    const leftDecorations = useRef<any[]>([]);
    const rightDecorations = useRef<any[]>([]);

    const computeDecorations = useCallback((monacoInstance: any) => {
        if (!monacoInstance) return;

        const origLines = original.split('\n');
        const modLines = modified.split('\n');

        const leftDecs: any[] = [];
        const rightDecs: any[] = [];

        // Simple line-by-line comparison for decoration coloring
        const maxLen = Math.max(origLines.length, modLines.length);
        for (let i = 0; i < maxLen; i++) {
            const origLine = origLines[i];
            const modLine = modLines[i];

            if (origLine === undefined && modLine !== undefined) {
                // Added line
                rightDecs.push({
                    range: new monacoInstance.Range(i + 1, 1, i + 1, 1),
                    options: {
                        isWholeLine: true,
                        className: 'diff-line-added',
                        linesDecorationsClassName: 'diff-glyph-added',
                    }
                });
            } else if (modLine === undefined && origLine !== undefined) {
                // Removed line
                leftDecs.push({
                    range: new monacoInstance.Range(i + 1, 1, i + 1, 1),
                    options: {
                        isWholeLine: true,
                        className: 'diff-line-removed',
                        linesDecorationsClassName: 'diff-glyph-removed',
                    }
                });
            } else if (origLine !== modLine) {
                // Changed line
                leftDecs.push({
                    range: new monacoInstance.Range(i + 1, 1, i + 1, 1),
                    options: {
                        isWholeLine: true,
                        className: 'diff-line-removed',
                        linesDecorationsClassName: 'diff-glyph-removed',
                    }
                });
                rightDecs.push({
                    range: new monacoInstance.Range(i + 1, 1, i + 1, 1),
                    options: {
                        isWholeLine: true,
                        className: 'diff-line-added',
                        linesDecorationsClassName: 'diff-glyph-added',
                    }
                });
            }
        }

        leftDecorations.current = leftDecs;
        rightDecorations.current = rightDecs;
    }, [original, modified]);

    return (
        <div className="flex-1 flex flex-col overflow-hidden">
            {/* Labels */}
            <div className="flex border-b border-border shrink-0">
                <div className="flex-1 px-3 py-1.5 text-xs font-medium text-red-400 bg-red-500/10 border-r border-border">
                    Current (deployed)
                </div>
                <div className="flex-1 px-3 py-1.5 text-xs font-medium text-green-400 bg-green-500/10">
                    Proposed (dry-run)
                </div>
            </div>
            {/* Editors */}
            <div className="flex-1 flex overflow-hidden">
                <div className="flex-1 border-r border-border">
                    <Editor
                        height="100%"
                        language="yaml"
                        value={original}
                        theme="vs-dark"
                        onMount={(editor, monaco) => {
                            leftEditorRef.current = editor;
                            computeDecorations(monaco);
                            editor.createDecorationsCollection(leftDecorations.current);
                            editor.onDidScrollChange(() => syncScroll('left'));
                        }}
                        options={{
                            readOnly: true,
                            minimap: { enabled: false },
                            scrollBeyondLastLine: false,
                            fontSize: 12,
                            lineNumbers: 'on',
                            renderLineHighlight: 'none',
                            automaticLayout: true,
                            wordWrap: 'off',
                            folding: true,
                            foldingStrategy: 'indentation',
                            glyphMargin: true,
                        }}
                    />
                </div>
                <div className="flex-1">
                    <Editor
                        height="100%"
                        language="yaml"
                        value={modified}
                        theme="vs-dark"
                        onMount={(editor, monaco) => {
                            rightEditorRef.current = editor;
                            computeDecorations(monaco);
                            editor.createDecorationsCollection(rightDecorations.current);
                            editor.onDidScrollChange(() => syncScroll('right'));
                        }}
                        options={{
                            readOnly: true,
                            minimap: { enabled: false },
                            scrollBeyondLastLine: false,
                            fontSize: 12,
                            lineNumbers: 'on',
                            renderLineHighlight: 'none',
                            automaticLayout: true,
                            wordWrap: 'off',
                            folding: true,
                            foldingStrategy: 'indentation',
                            glyphMargin: true,
                        }}
                    />
                </div>
            </div>
        </div>
    );
}

// Simple diff summary: count lines added, removed, changed
function computeDiffSummary(original: string, modified: string) {
    const origLines = original.split('\n');
    const modLines = modified.split('\n');

    let added = 0;
    let removed = 0;
    let changed = 0;

    const maxLen = Math.max(origLines.length, modLines.length);
    for (let i = 0; i < maxLen; i++) {
        const origLine = origLines[i];
        const modLine = modLines[i];

        if (origLine === undefined && modLine !== undefined) added++;
        else if (modLine === undefined && origLine !== undefined) removed++;
        else if (origLine !== modLine) changed++;
    }

    return { added, removed, changed };
}
