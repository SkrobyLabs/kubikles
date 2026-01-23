// Configuration schema - single source of truth for property metadata
export const configSchema = {
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
    metrics: {
        _meta: { label: 'Kubernetes Metrics', description: 'CPU/Memory metrics from Kubernetes metrics-server' },
        pollIntervalMs: {
            type: 'number',
            label: 'Poll Interval',
            description: 'How often to refresh CPU/Memory metrics in resource list views',
            min: 5000,
            max: 300000,
            step: 1000,
            unit: 'ms',
            default: 30000
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
    ui: {
        _meta: { label: 'UI', description: 'User interface settings' },
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
        scrollZoomEnabled: {
            type: 'boolean',
            label: 'Scroll Zoom',
            description: 'Enable Cmd/Ctrl+Scroll to zoom in/out',
            default: false
        },
        showTabIcons: {
            type: 'boolean',
            label: 'Show Tab Icons',
            description: 'Display resource type icons in tab titles',
            default: true
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
                if (currentValue !== undefined && currentValue !== fieldSchema.default) {
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
