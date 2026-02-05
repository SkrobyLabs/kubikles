// Configuration schema - single source of truth for property metadata
export const configSchema = {
    app: {
        _meta: { label: 'App', description: 'Application information and diagnostics' },
        crashLogPath: {
            type: 'readonly',
            label: 'Crash Log Location',
            description: 'Log file for debugging crashes and errors',
            asyncSource: 'crashLogPath',
            showCopy: true,
            showOpenFolder: true,
            action: 'openCrashLogDir'
        }
    },
    logs: {
        _meta: { label: 'Logs', description: 'Log viewer settings' },
        lineWrap: {
            type: 'boolean',
            label: 'Line Wrap',
            description: 'Wrap long lines in the log viewer',
            default: true
        },
        showTimestamps: {
            type: 'boolean',
            label: 'Show Timestamps',
            description: 'Display timestamps in the log viewer',
            default: false
        },
        position: {
            type: 'enum',
            label: 'Initial Position',
            description: 'Where to start when opening logs',
            options: [
                { value: 'start', label: 'Start' },
                { value: 'end', label: 'End' },
                { value: 'all', label: 'All' }
            ],
            default: 'end'
        },
        search: {
            _meta: { label: 'Search', isNested: true },
            debounceMs: {
                type: 'number',
                label: 'Debounce',
                description: 'Delay before search triggers in search-as-you-type mode',
                min: 0,
                max: 2000,
                step: 50,
                unit: 'ms',
                default: 200
            },
            searchOnEnter: {
                type: 'boolean',
                label: 'Search on Enter',
                description: 'Only search when pressing Enter (otherwise search as you type)',
                default: true
            },
            useRegex: {
                type: 'boolean',
                label: 'Use Regex',
                description: 'Enable regex matching by default',
                default: false
            },
            filterOnly: {
                type: 'boolean',
                label: 'Filter Only',
                description: 'Show only matching lines by default',
                default: true
            },
            contextLinesBefore: {
                type: 'number',
                label: 'Context Lines Before',
                description: 'Lines to show before matches when filtering',
                min: 0,
                max: 20,
                step: 1,
                default: 1
            },
            contextLinesAfter: {
                type: 'number',
                label: 'Context Lines After',
                description: 'Lines to show after matches when filtering',
                min: 0,
                max: 20,
                step: 1,
                default: 5
            }
        }
    },
    kubernetes: {
        _meta: { label: 'Kubernetes', description: 'Kubernetes API and metrics settings' },
        apiTimeoutMs: {
            type: 'number',
            label: 'API Timeout',
            description: 'Timeout for Kubernetes API requests. Increase for slow clusters.',
            min: 10000,
            max: 300000,
            step: 5000,
            unit: 'ms',
            default: 60000
        },
        metricsPollIntervalMs: {
            type: 'number',
            label: 'Metrics Poll Interval',
            description: 'How often to refresh CPU/Memory metrics in resource list views',
            min: 5000,
            max: 300000,
            step: 1000,
            unit: 'ms',
            default: 30000
        },
        connectionTestTimeoutSeconds: {
            type: 'number',
            label: 'Connection Test Timeout',
            description: 'When switching contexts, a connectivity check runs first to fail fast if cluster is unreachable',
            subtext: 'Increase for high-latency clusters. Decrease for faster failure detection.',
            min: 1,
            max: 30,
            step: 1,
            unit: 's',
            default: 5
        },
        nodeDebugImage: {
            type: 'string',
            label: 'Node Debug Shell Image',
            description: 'Default container image for node debug shell sessions',
            placeholder: 'e.g., alpine:latest, nicolaka/netshoot:latest',
            default: 'alpine:latest'
        }
    },
    performance: {
        _meta: { label: 'Performance', description: 'Performance panel settings' },
        pollIntervalMs: {
            type: 'number',
            label: 'Poll Interval',
            description: 'How often to refresh performance data',
            min: 500,
            max: 10000,
            step: 100,
            unit: 'ms',
            default: 1500
        },
        eventCoalescerMs: {
            type: 'number',
            label: 'Event Coalescer',
            description: 'Frame interval for resource event batching (lower = more responsive)',
            min: 1,
            max: 100,
            step: 1,
            unit: 'ms',
            default: 16
        },
        enableRequestCancellation: {
            type: 'boolean',
            label: 'Enable Request Cancellation',
            description: 'Cancel in-flight API requests when navigating away',
            subtext: 'Disable if experiencing slow navigation. Works around Go HTTP/2 bug where cancellation causes performance collapse.',
            default: true
        },
        forceHttp1: {
            type: 'boolean',
            label: 'Force HTTP/1.1',
            description: 'Use HTTP/1.1 instead of HTTP/2',
            subtext: 'Opens multiple TCP connections for parallel requests, avoiding HTTP/2 flow control bottlenecks. Requires context switch.',
            default: false
        },
        clientPoolSize: {
            type: 'number',
            label: 'Additional Connections',
            description: 'Extra K8s client connections for better parallelism (0 = just main connection). Requires context switch.',
            min: 0,
            max: 10,
            step: 1,
            default: 0
        }
    },
    portForwards: {
        _meta: { label: 'Port Forwards', description: 'Port forwarding settings' },
        autoStartMode: {
            type: 'enum',
            label: 'Auto-Start Mode',
            description: 'Which port forwards to restore on app launch',
            options: [
                { value: 'all', label: 'All' },
                { value: 'favorites', label: 'Favorites Only' },
                { value: 'none', label: 'None' }
            ],
            default: 'favorites'
        }
    },
    ai: {
        _meta: { label: 'AI', description: 'AI assistant settings' },
        model: {
            type: 'enum',
            label: 'Model',
            description: 'AI model to use for chat responses',
            options: [
                { value: 'sonnet', label: 'Sonnet (Fast)' },
                { value: 'opus', label: 'Opus (Smart)' },
                { value: 'haiku', label: 'Haiku (Fastest)' },
            ],
            default: 'sonnet'
        },
        requestTimeout: {
            type: 'number',
            label: 'Request Timeout',
            description: 'Maximum time for an AI request before it is cancelled',
            min: 1,
            max: 60,
            step: 1,
            unit: 'min',
            default: 10
        },
        panelWidth: {
            type: 'number',
            label: 'Panel Width',
            description: 'Width of the AI assistant panel',
            min: 280,
            max: 800,
            step: 20,
            unit: 'px',
            default: 384
        },
        allowedTools: {
            type: 'checkboxGroup',
            label: 'Allowed Tools',
            description: 'Tools the AI can use. Cluster read tools are enabled by default. Dangerous tools (Bash, WebSearch) are disabled by default. Add external MCP tool names in the input below.',
            options: [
                { value: 'get_pod_logs', label: 'Get Pod Logs' },
                { value: 'get_resource_yaml', label: 'Get Resource YAML' },
                { value: 'list_resources', label: 'List Resources' },
                { value: 'get_events', label: 'Get Events' },
                { value: 'describe_resource', label: 'Describe Resource' },
                { value: 'list_crds', label: 'List CRDs' },
                { value: 'list_custom_resources', label: 'List Custom Resources' },
                { value: 'get_custom_resource_yaml', label: 'Get Custom Resource YAML' },
                { value: 'get_cluster_metrics', label: 'Get Cluster Metrics' },
                { value: 'get_pod_metrics', label: 'Get Pod Metrics' },
                { value: 'get_namespace_summary', label: 'Get Namespace Summary' },
                { value: 'get_resource_dependencies', label: 'Get Resource Dependencies' },
                { value: 'Bash', label: 'Bash (run shell commands)', warn: true },
                { value: 'WebSearch', label: 'Web Search', warn: true },
                { value: 'Read', label: 'Read Files', warn: true },
                { value: 'Write', label: 'Write Files', warn: true },
            ],
            default: [
                'get_pod_logs', 'get_resource_yaml', 'list_resources',
                'get_events', 'describe_resource', 'list_crds',
                'list_custom_resources', 'get_custom_resource_yaml',
                'get_cluster_metrics', 'get_pod_metrics',
                'get_namespace_summary', 'get_resource_dependencies'
            ]
        }
    },
    ui: {
        _meta: { label: 'UI', description: 'User interface settings' },
        theme: {
            type: 'enum',
            label: 'Theme',
            description: 'Color theme for the application',
            source: 'theme',
            optionsSource: 'themes',
            default: 'default-dark'
        },
        zoomLevel: {
            type: 'number',
            label: 'Zoom Level',
            description: 'UI zoom level (1.0 = 100%)',
            source: 'zoom',
            min: 0.5,
            max: 2.0,
            step: 0.05,
            default: 1.0
        },
        scrollZoomEnabled: {
            type: 'boolean',
            label: 'Scroll Zoom',
            description: 'Enable Cmd/Ctrl+Scroll to zoom in/out',
            default: false
        },
        fonts: {
            _meta: { label: 'Fonts', isNested: true },
            uiFont: {
                type: 'enum',
                label: 'UI Font',
                description: 'Font for the user interface',
                source: 'theme', // Special marker - value comes from ThemeContext
                optionsSource: 'uiFonts',
                default: 'inter'
            },
            monoFont: {
                type: 'enum',
                label: 'Monospace Font',
                description: 'Font for code, logs, and terminals',
                source: 'theme',
                optionsSource: 'monoFonts',
                default: 'jetbrains'
            }
        },
        searchDebounceMs: {
            type: 'number',
            label: 'Search Debounce',
            description: 'Delay before resource list search triggers',
            min: 0,
            max: 1000,
            step: 50,
            unit: 'ms',
            default: 150
        },
        copyFeedbackMs: {
            type: 'number',
            label: 'Copy Feedback Duration',
            description: 'How long "Copied!" feedback shows',
            min: 500,
            max: 5000,
            step: 250,
            unit: 'ms',
            default: 2000
        },
        showTabIcons: {
            type: 'boolean',
            label: 'Show Tab Icons',
            description: 'Display resource type icons in tab titles',
            default: true
        }
    },
    debug: {
        _meta: { label: 'Debug', description: 'Developer and debugging options' },
        showLogSourceMarkers: {
            type: 'boolean',
            label: 'Show Log Source Markers',
            description: 'Show debug download button in log viewer',
            subtext: 'Downloads logs with markers showing how each line was fetched: [INITIAL], [STREAM], [BEFORE], [AFTER]. Useful for debugging log viewer pagination.',
            default: false
        }
    }
};

// Get sections sorted alphabetically
export const getSortedSections = () =>
    Object.keys(configSchema).sort();

// Get a field's schema by path (e.g., "logs.search.debounceMs")
export const getFieldSchema = (path) => {
    const parts = path.split('.');
    let current = configSchema;
    for (const part of parts) {
        if (!current || !current[part]) return null;
        current = current[part];
    }
    return current;
};

// Check if a value differs from default
export const isModified = (path, value) => {
    const schema = getFieldSchema(path);
    if (!schema || schema.default === undefined) return false;
    if (Array.isArray(schema.default)) {
        return JSON.stringify([...(value || [])].sort()) !== JSON.stringify([...schema.default].sort());
    }
    return value !== schema.default;
};

// Get all modified fields with their current and default values
export const getModifiedFields = (config) => {
    const modified = [];

    const checkFields = (schema, configObj, path = '') => {
        for (const [key, fieldSchema] of Object.entries(schema)) {
            if (key === '_meta') continue;

            const currentPath = path ? `${path}.${key}` : key;
            const currentValue = configObj?.[key];

            // Check if this is a nested group
            if (fieldSchema._meta?.isNested) {
                checkFields(fieldSchema, currentValue, currentPath);
                continue;
            }

            // Check if field has a type (is a real field)
            if (fieldSchema.type && fieldSchema.default !== undefined) {
                let isDifferent;
                if (Array.isArray(fieldSchema.default)) {
                    isDifferent = currentValue !== undefined &&
                        JSON.stringify([...(currentValue || [])].sort()) !== JSON.stringify([...fieldSchema.default].sort());
                } else {
                    isDifferent = currentValue !== undefined && currentValue !== fieldSchema.default;
                }
                if (isDifferent) {
                    modified.push({
                        path: currentPath,
                        label: fieldSchema.label || key,
                        currentValue,
                        defaultValue: fieldSchema.default
                    });
                }
            }
        }
    };

    for (const [sectionKey, sectionSchema] of Object.entries(configSchema)) {
        checkFields(sectionSchema, config?.[sectionKey], sectionKey);
    }

    return modified;
};

// Search fields by label or description, returns { section: [fieldKeys] }
export const searchFields = (query) => {
    if (!query || query.trim() === '') return null;

    const lowerQuery = query.toLowerCase();
    const results = {};

    const searchInObject = (obj, sectionKey, prefix = '') => {
        for (const [key, value] of Object.entries(obj)) {
            if (key === '_meta') continue;

            // Check if this is a nested group
            if (value._meta?.isNested) {
                searchInObject(value, sectionKey, `${prefix}${key}.`);
                continue;
            }

            // Check if field matches
            if (value.label || value.description) {
                const label = (value.label || '').toLowerCase();
                const description = (value.description || '').toLowerCase();
                const fieldKey = key.toLowerCase();

                if (label.includes(lowerQuery) || description.includes(lowerQuery) || fieldKey.includes(lowerQuery)) {
                    if (!results[sectionKey]) results[sectionKey] = [];
                    results[sectionKey].push(`${prefix}${key}`);
                }
            }
        }
    };

    for (const sectionKey of Object.keys(configSchema)) {
        // Also check section label
        const sectionLabel = (configSchema[sectionKey]._meta?.label || '').toLowerCase();
        if (sectionLabel.includes(lowerQuery)) {
            // Section matches - include all its fields
            results[sectionKey] = ['*'];
        } else {
            searchInObject(configSchema[sectionKey], sectionKey);
        }
    }

    return Object.keys(results).length > 0 ? results : null;
};
