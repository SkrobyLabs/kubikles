import React, { useState, useRef, useEffect, useMemo } from 'react';
import Editor from '@monaco-editor/react';
import { useConfig } from '~/context';
import { XMarkIcon, ArrowPathIcon } from '@heroicons/react/24/outline';
import ConfigEditorUI from './config-editor/ConfigEditorUI';
import { configSchema } from '~/config/configSchema';

// Build flat description map from configSchema (single source of truth).
// Walks the schema tree and collects "path → description" for every typed field.
function buildDescriptions(schema, prefix = '') {
    const map = {};
    for (const [key, value] of Object.entries(schema)) {
        if (key === '_meta') continue;
        const fullKey = prefix ? `${prefix}.${key}` : key;
        if (value._meta?.isNested) {
            Object.assign(map, buildDescriptions(value, fullKey));
        } else if (value.type && value.description) {
            map[fullKey] = value.description;
        } else if (!value.type && typeof value === 'object') {
            Object.assign(map, buildDescriptions(value, fullKey));
        }
    }
    return map;
}
const configDescriptions = buildDescriptions(configSchema);

// Convert nested object to flat key=value format with comments
const flattenConfig = (obj, prefix = '') => {
    const lines = [];
    for (const key in obj) {
        const fullKey = prefix ? `${prefix}.${key}` : key;
        const value = obj[key];
        if (value && typeof value === 'object' && !Array.isArray(value)) {
            lines.push(...flattenConfig(value, fullKey));
        } else if (value !== undefined) {
            // Add description comment if available
            const description = configDescriptions[fullKey];
            if (description) {
                lines.push(`# ${description}`);
            }
            // Format value based on type
            let formattedValue;
            if (typeof value === 'string') {
                formattedValue = `"${value}"`;
            } else if (Array.isArray(value)) {
                // Serialize arrays as JSON to preserve structure
                formattedValue = JSON.stringify(value);
            } else {
                formattedValue = String(value);
            }
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
        } else if (trimmedValue.startsWith('[') || trimmedValue.startsWith('{')) {
            // Parse JSON arrays and objects
            try {
                value = JSON.parse(trimmedValue);
            } catch {
                value = trimmedValue;
            }
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

    const [mode, setMode] = useState('ui'); // 'ui', 'flat', or 'json'
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

    // Initialize content when switching to text editor modes
    useEffect(() => {
        if (mode !== 'ui') {
            setContent(generateContent(config, mode));
        }
    }, []);



    // Switch between modes
    const switchMode = (newMode) => {
        if (newMode === mode) return;

        setError('');

        // When switching from UI mode, generate content from current config
        if (mode === 'ui') {
            setContent(generateContent(config, newMode));
            setMode(newMode);
            return;
        }

        // When switching to UI mode, parse and save current content first
        if (newMode === 'ui') {
            try {
                let parsed;
                if (mode === 'flat') {
                    parsed = unflattenConfig(content);
                } else {
                    parsed = JSON.parse(content);
                }
                updateConfig(parsed);
                setMode(newMode);
            } catch (e) {
                setError(`Cannot switch mode: ${e.message}`);
            }
            return;
        }

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
                                },
                                scrollZoomEnabled: {
                                    type: 'boolean',
                                    description: 'Enable Cmd/Ctrl+Scroll to zoom in/out',
                                    default: true
                                },
                                showTabIcons: {
                                    type: 'boolean',
                                    description: 'Display resource type icons in tab titles',
                                    default: true
                                }
                            }
                        },
                        kubernetes: {
                            type: 'object',
                            description: 'Kubernetes API settings',
                            properties: {
                                apiTimeoutMs: {
                                    type: 'number',
                                    description: 'API request timeout in milliseconds. Increase for slow clusters.',
                                    default: 60000,
                                    minimum: 10000,
                                    maximum: 300000
                                },
                                metricsPollIntervalMs: {
                                    type: 'number',
                                    description: 'Poll interval in milliseconds for Kubernetes CPU/Memory metrics',
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
                                },
                                eventCoalescerMs: {
                                    type: 'number',
                                    description: 'Frame interval in milliseconds for resource event batching',
                                    default: 16,
                                    minimum: 1,
                                    maximum: 100
                                },
                                enableRequestCancellation: {
                                    type: 'boolean',
                                    description: 'Cancel in-flight API requests when navigating. Disable if experiencing slow navigation (Go HTTP/2 bug workaround).',
                                    default: false
                                },
                                forceHttp1: {
                                    type: 'boolean',
                                    description: 'Use HTTP/1.1 instead of HTTP/2. Opens multiple connections for parallel requests. Requires context switch.',
                                    default: false
                                },
                                clientPoolSize: {
                                    type: 'number',
                                    description: 'Additional K8s client connections for parallelism (0 = just main connection). Requires context switch.',
                                    default: 0,
                                    minimum: 0,
                                    maximum: 10
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
            if (mode !== 'ui') {
                setContent(generateContent(defaultConfig, mode));
            }
            setError('');
        }
    };

    // Determine editor language based on mode
    const editorLanguage = mode === 'json' ? 'json' : 'ini';

    // Render UI mode
    if (mode === 'ui') {
        return <ConfigEditorUI onSwitchMode={switchMode} />;
    }

    // Render text editor modes (flat/json)
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
                            onClick={() => switchMode('ui')}
                            className="px-3 py-1.5 transition-colors text-gray-400 hover:text-white"
                        >
                            UI
                        </button>
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
