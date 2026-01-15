import React, { useState, useRef, useEffect, useMemo } from 'react';
import Editor from '@monaco-editor/react';
import { useConfig } from '../../context/ConfigContext';
import { XMarkIcon, ArrowPathIcon } from '@heroicons/react/24/outline';

// Descriptions for config keys (used in flat mode comments)
const configDescriptions = {
    'logs.lineWrap': 'If true, wrap long lines in log viewer',
    'logs.showTimestamps': 'If true, show timestamps in log viewer',
    'logs.position': 'Initial log position: "start", "end", or "all"',
    'logs.search.debounceMs': 'Debounce delay in milliseconds for search-as-you-type mode',
    'logs.search.searchOnEnter': 'If true, search only triggers on Enter key. If false, search as you type.',
    'logs.search.useRegex': 'If true, search uses regex matching by default',
    'logs.search.filterOnly': 'If true, show only matching lines by default',
    'logs.search.contextLinesBefore': 'Number of context lines to show before matches when filtering',
    'logs.search.contextLinesAfter': 'Number of context lines to show after matches when filtering',
    'portForwards.autoStartMode': 'Auto-start mode: "all" (start all that were running), "favorites" (only favorites), "none" (disabled)',
    'ui.searchDebounceMs': 'Debounce delay in milliseconds for resource list search',
    'ui.copyFeedbackMs': 'How long "Copied!" feedback shows in milliseconds',
    'metrics.pollIntervalMs': 'Poll interval in milliseconds for node/pod metrics (default: 30000)',
    'performance.pollIntervalMs': 'Poll interval in milliseconds for performance panel (default: 1500)'
};

// Convert nested object to flat key=value format with comments
const flattenConfig = (obj, prefix = '') => {
    const lines = [];
    for (const key in obj) {
        const fullKey = prefix ? `${prefix}.${key}` : key;
        const value = obj[key];
        if (value && typeof value === 'object' && !Array.isArray(value)) {
            lines.push(...flattenConfig(value, fullKey));
        } else {
            // Add description comment if available
            const description = configDescriptions[fullKey];
            if (description) {
                lines.push(`# ${description}`);
            }
            // Format value based on type
            const formattedValue = typeof value === 'string' ? `"${value}"` : String(value);
            lines.push(`${fullKey} = ${formattedValue}`);
        }
    }
    return lines;
};

// Convert flat key=value format back to nested object
const unflattenConfig = (text) => {
    const result = {};
    const lines = text.split('\n');

    for (const line of lines) {
        const trimmed = line.trim();
        // Skip empty lines and comments
        if (!trimmed || trimmed.startsWith('#') || trimmed.startsWith('//')) {
            continue;
        }

        const match = trimmed.match(/^([a-zA-Z0-9_.]+)\s*=\s*(.+)$/);
        if (!match) {
            throw new Error(`Invalid line: ${trimmed}`);
        }

        const [, key, rawValue] = match;
        let value;

        // Parse value
        const trimmedValue = rawValue.trim();
        if (trimmedValue === 'true') {
            value = true;
        } else if (trimmedValue === 'false') {
            value = false;
        } else if (trimmedValue === 'null') {
            value = null;
        } else if (/^-?\d+$/.test(trimmedValue)) {
            value = parseInt(trimmedValue, 10);
        } else if (/^-?\d*\.\d+$/.test(trimmedValue)) {
            value = parseFloat(trimmedValue);
        } else if ((trimmedValue.startsWith('"') && trimmedValue.endsWith('"')) ||
                   (trimmedValue.startsWith("'") && trimmedValue.endsWith("'"))) {
            value = trimmedValue.slice(1, -1);
        } else {
            // Treat as unquoted string
            value = trimmedValue;
        }

        // Set nested value
        const parts = key.split('.');
        let current = result;
        for (let i = 0; i < parts.length - 1; i++) {
            if (!current[parts[i]]) {
                current[parts[i]] = {};
            }
            current = current[parts[i]];
        }
        current[parts[parts.length - 1]] = value;
    }

    return result;
};

export default function ConfigEditor() {
    const {
        config,
        getConfigJson,
        updateConfig,
        resetConfig,
        defaultConfig,
        closeConfigEditor
    } = useConfig();

    const [mode, setMode] = useState('flat'); // 'flat' or 'json'
    const [content, setContent] = useState('');
    const [error, setError] = useState('');
    const [saved, setSaved] = useState(false);
    const editorRef = useRef(null);
    const monacoRef = useRef(null);

    // Generate content based on mode
    const generateContent = (cfg, targetMode) => {
        if (targetMode === 'flat') {
            return flattenConfig(cfg).join('\n');
        }
        return JSON.stringify(cfg, null, 2);
    };

    // Initialize content
    useEffect(() => {
        setContent(generateContent(config, mode));
    }, []);



    // Switch between modes
    const switchMode = (newMode) => {
        if (newMode === mode) return;

        setError('');

        try {
            // Parse current content first
            let parsed;
            if (mode === 'flat') {
                parsed = unflattenConfig(content);
            } else {
                parsed = JSON.parse(content);
            }

            // Convert to new format
            setContent(generateContent(parsed, newMode));
            setMode(newMode);
        } catch (e) {
            setError(`Cannot switch mode: ${e.message}`);
        }
    };

    const handleEditorDidMount = (editor, monaco) => {
        editorRef.current = editor;
        monacoRef.current = monaco;

        // Configure JSON schema for auto-completion (only applies in JSON mode)
        monaco.languages.json.jsonDefaults.setDiagnosticsOptions({
            validate: true,
            allowComments: false,
            schemas: [{
                uri: 'kubikles://settings',
                fileMatch: ['*'],
                schema: {
                    type: 'object',
                    properties: {
                        logs: {
                            type: 'object',
                            description: 'Log viewer settings',
                            properties: {
                                lineWrap: {
                                    type: 'boolean',
                                    description: 'If true, wrap long lines in log viewer',
                                    default: true
                                },
                                showTimestamps: {
                                    type: 'boolean',
                                    description: 'If true, show timestamps in log viewer',
                                    default: false
                                },
                                position: {
                                    type: 'string',
                                    description: 'Initial log position',
                                    enum: ['start', 'end', 'all'],
                                    default: 'end'
                                },
                                search: {
                                    type: 'object',
                                    description: 'Search settings',
                                    properties: {
                                        debounceMs: {
                                            type: 'number',
                                            description: 'Debounce delay in milliseconds for search-as-you-type mode',
                                            default: 200,
                                            minimum: 0,
                                            maximum: 2000
                                        },
                                        searchOnEnter: {
                                            type: 'boolean',
                                            description: 'If true, search only triggers on Enter key. If false, search as you type.',
                                            default: true
                                        },
                                        useRegex: {
                                            type: 'boolean',
                                            description: 'If true, search uses regex matching by default',
                                            default: false
                                        },
                                        filterOnly: {
                                            type: 'boolean',
                                            description: 'If true, show only matching lines by default',
                                            default: false
                                        },
                                        contextLinesBefore: {
                                            type: 'number',
                                            description: 'Number of context lines to show before matches when filtering',
                                            default: 1,
                                            minimum: 0
                                        },
                                        contextLinesAfter: {
                                            type: 'number',
                                            description: 'Number of context lines to show after matches when filtering',
                                            default: 5,
                                            minimum: 0
                                        }
                                    }
                                }
                            }
                        },
                        portForwards: {
                            type: 'object',
                            description: 'Port forwarding settings',
                            properties: {
                                autoStartMode: {
                                    type: 'string',
                                    description: 'Auto-start mode: "all" (start all that were running), "favorites" (only favorites), "none" (disabled)',
                                    enum: ['all', 'favorites', 'none'],
                                    default: 'favorites'
                                }
                            }
                        },
                        ui: {
                            type: 'object',
                            description: 'UI timing settings',
                            properties: {
                                searchDebounceMs: {
                                    type: 'number',
                                    description: 'Debounce delay in milliseconds for resource list search',
                                    default: 150,
                                    minimum: 0,
                                    maximum: 1000
                                },
                                copyFeedbackMs: {
                                    type: 'number',
                                    description: 'How long "Copied!" feedback shows in milliseconds',
                                    default: 2000,
                                    minimum: 500,
                                    maximum: 5000
                                }
                            }
                        },
                        metrics: {
                            type: 'object',
                            description: 'Metrics polling settings',
                            properties: {
                                pollIntervalMs: {
                                    type: 'number',
                                    description: 'Poll interval in milliseconds for node/pod metrics',
                                    default: 30000,
                                    minimum: 5000,
                                    maximum: 300000
                                }
                            }
                        },
                        performance: {
                            type: 'object',
                            description: 'Performance panel settings',
                            properties: {
                                pollIntervalMs: {
                                    type: 'number',
                                    description: 'Poll interval in milliseconds for performance panel',
                                    default: 1500,
                                    minimum: 500,
                                    maximum: 10000
                                }
                            }
                        }
                    }
                }
            }]
        });

        editor.updateOptions({
            minimap: { enabled: false },
            scrollBeyondLastLine: false,
            fontSize: 13,
            lineNumbers: 'on',
            folding: true,
            renderWhitespace: 'selection',
            wordWrap: 'on',
        });

        // Add Cmd+S / Ctrl+S save shortcut
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
            handleSave();
        });
    };

    const handleSave = () => {
        setError('');
        setSaved(false);

        try {
            let parsed;
            if (mode === 'flat') {
                parsed = unflattenConfig(content);
            } else {
                parsed = JSON.parse(content);
            }
            updateConfig(parsed);
            setSaved(true);
            setTimeout(() => setSaved(false), 2000);
        } catch (e) {
            setError(`Invalid ${mode === 'flat' ? 'config' : 'JSON'}: ${e.message}`);
        }
    };

    const handleReset = () => {
        if (confirm('Reset configuration to defaults?')) {
            resetConfig();
            setContent(generateContent(defaultConfig, mode));
            setError('');
        }
    };

    // Determine editor language based on mode
    const editorLanguage = mode === 'json' ? 'json' : 'ini';

    return (
        <div className="h-full flex flex-col bg-background">
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0 titlebar-drag">
                <h2 className="text-sm font-semibold text-text">Settings</h2>
                <div className="flex items-center gap-3">
                    {/* Error/Success Message */}
                    {error && (
                        <span className="text-sm text-red-400">{error}</span>
                    )}
                    {saved && (
                        <span className="text-sm text-green-400">Settings saved!</span>
                    )}

                    {/* Mode Toggle */}
                    <div className="flex items-center bg-background rounded overflow-hidden text-xs">
                        <button
                            onClick={() => switchMode('flat')}
                            className={`px-3 py-1.5 transition-colors ${mode === 'flat' ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}`}
                        >
                            Flat
                        </button>
                        <button
                            onClick={() => switchMode('json')}
                            className={`px-3 py-1.5 transition-colors ${mode === 'json' ? 'bg-primary text-white' : 'text-gray-400 hover:text-white'}`}
                        >
                            JSON
                        </button>
                    </div>

                    <div className="w-px h-5 bg-border" />

                    {/* Actions */}
                    <button
                        onClick={handleReset}
                        className="flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                    >
                        <ArrowPathIcon className="w-4 h-4" />
                        Reset
                    </button>
                    <button
                        onClick={handleSave}
                        className="px-4 py-1.5 text-sm font-medium bg-primary text-white rounded hover:bg-blue-600 transition-colors"
                    >
                        Save
                    </button>
                    <button
                        onClick={closeConfigEditor}
                        className="p-1.5 rounded text-gray-400 hover:text-white hover:bg-white/10 transition-colors"
                    >
                        <XMarkIcon className="w-5 h-5" />
                    </button>
                </div>
            </div>

            {/* Editor */}
            <div className="flex-1 overflow-hidden">
                <Editor
                    height="100%"
                    language={editorLanguage}
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
        </div>
    );
}
