import React, { useEffect, useState, useCallback } from 'react';
import ConfigField from './fields/ConfigField';
import ConfigFieldGroup from './ConfigFieldGroup';
import { configSchema, isModified } from '~/config/configSchema';
import { GetCrashLogPath, OpenCrashLogDir } from 'wailsjs/go/main/App';
import { useTheme } from '~/context';

// Async value sources
const asyncSources = {
    crashLogPath: GetCrashLogPath
};

// Action handlers
const actionHandlers = {
    openCrashLogDir: OpenCrashLogDir
};

// Zoom constants (must match App.jsx)
const ZOOM_STORAGE_KEY = 'kubikles-zoom-level';
const ZOOM_DEFAULT = 1.0;

export default function ConfigSection({ section, config, onFieldChange, searchResults }) {
    const sectionSchema = configSchema[section];
    const [asyncValues, setAsyncValues] = useState({});
    const { currentTheme, themes, switchTheme } = useTheme();

    // Zoom level state (reads from localStorage, updates body style)
    const [zoomLevel, setZoomLevelState] = useState(() => {
        const saved = localStorage.getItem(ZOOM_STORAGE_KEY);
        return saved ? parseFloat(saved) : ZOOM_DEFAULT;
    });

    const setZoomLevel = useCallback((value) => {
        const numValue = parseFloat(value);
        setZoomLevelState(numValue);
        document.body.style.zoom = numValue;
        localStorage.setItem(ZOOM_STORAGE_KEY, numValue.toString());
    }, []);

    // Listen for zoom changes from scroll/other sources
    useEffect(() => {
        const handleZoomChanged = (e) => {
            setZoomLevelState(e.detail);
        };
        window.addEventListener('zoom:changed', handleZoomChanged);
        return () => window.removeEventListener('zoom:changed', handleZoomChanged);
    }, []);

    // Map for theme-sourced values and setters (for top-level fields)
    const themeValues = { theme: currentTheme?.id };
    const themeSetters = { theme: switchTheme };
    const themeOptions = { themes: themes || [] };

    // Map for zoom-sourced values and setters
    const zoomValues = { zoomLevel };
    const zoomSetters = { zoomLevel: setZoomLevel };

    // Load async values for readonly fields
    useEffect(() => {
        if (!sectionSchema) return;

        const loadAsyncValues = async () => {
            const { _meta, ...fields } = sectionSchema;
            const newValues = {};

            for (const [key, schema] of Object.entries(fields)) {
                if (schema.asyncSource && asyncSources[schema.asyncSource]) {
                    try {
                        newValues[key] = await asyncSources[schema.asyncSource]();
                    } catch (err) {
                        console.error(`Failed to load async value for ${key}:`, err);
                        newValues[key] = 'Error loading value';
                    }
                }
            }

            if (Object.keys(newValues).length > 0) {
                setAsyncValues(newValues);
            }
        };

        loadAsyncValues();
    }, [section, sectionSchema]);

    const handleAction = useCallback((action) => {
        if (actionHandlers[action]) {
            actionHandlers[action]().catch(err => {
                console.error(`Action ${action} failed:`, err);
            });
        }
    }, []);

    if (!sectionSchema) return null;

    const { _meta, ...fields } = sectionSchema;

    // Get matching field keys for this section (if searching)
    const matchingFields = searchResults?.[section];
    const showAllFields = !searchResults || matchingFields?.[0] === '*';

    // Check if a field matches search
    const fieldMatches = (fieldKey, groupKey = null) => {
        if (showAllFields) return true;
        if (!matchingFields) return false;
        const fullKey = groupKey ? `${groupKey}.${fieldKey}` : fieldKey;
        return matchingFields.includes(fullKey);
    };

    // Check if any field in a group matches
    const groupHasMatches = (groupKey, groupFields) => {
        if (showAllFields) return true;
        if (!matchingFields) return false;
        return Object.keys(groupFields).some(fieldKey =>
            matchingFields.includes(`${groupKey}.${fieldKey}`)
        );
    };

    // Separate top-level fields from nested groups
    const topLevelFields = {};
    const nestedGroups = {};

    Object.entries(fields).forEach(([key, schema]) => {
        if (schema._meta?.isNested) {
            nestedGroups[key] = schema;
        } else {
            topLevelFields[key] = schema;
        }
    });

    // Filter top-level fields if searching
    const visibleTopLevelFields = Object.entries(topLevelFields).filter(
        ([key]) => fieldMatches(key)
    );

    // Filter nested groups if searching
    const visibleNestedGroups = Object.entries(nestedGroups).filter(
        ([groupKey, groupSchema]) => {
            const { _meta: _, ...groupFields } = groupSchema;
            return groupHasMatches(groupKey, groupFields);
        }
    );

    return (
        <div className="space-y-6">
            {/* Section header */}
            <div>
                <h3 className="text-lg font-semibold text-text">{_meta?.label || section}</h3>
                {_meta?.description && (
                    <p className="text-sm text-text-muted mt-1">{_meta.description}</p>
                )}
            </div>

            {/* Top-level fields */}
            {visibleTopLevelFields.length > 0 && (
                <div className="space-y-1">
                    {visibleTopLevelFields.map(([key, schema]) => {
                        const path = `${section}.${key}`;

                        // Handle theme-sourced fields (e.g., theme selector)
                        if (schema.source === 'theme') {
                            const value = themeValues[key];
                            const setter = themeSetters[key];
                            const options = themeOptions[schema.optionsSource]?.map(t => ({
                                value: t.id,
                                label: t.name
                            }));

                            return (
                                <ConfigField
                                    key={key}
                                    schema={{ ...schema, options }}
                                    value={value}
                                    onChange={setter}
                                    isModified={value !== schema.default}
                                />
                            );
                        }

                        // Handle zoom-sourced fields (e.g., zoom level)
                        if (schema.source === 'zoom') {
                            const value = zoomValues[key];
                            const setter = zoomSetters[key];

                            return (
                                <ConfigField
                                    key={key}
                                    schema={schema}
                                    value={value}
                                    onChange={setter}
                                    isModified={value !== schema.default}
                                />
                            );
                        }

                        const value = config?.[section]?.[key];

                        return (
                            <ConfigField
                                key={key}
                                schema={schema}
                                value={value}
                                onChange={(val) => onFieldChange(path, val)}
                                isModified={isModified(path, value)}
                                asyncValue={asyncValues[key]}
                                onAction={handleAction}
                            />
                        );
                    })}
                </div>
            )}

            {/* Nested groups */}
            {visibleNestedGroups.map(([groupKey, groupSchema]) => {
                const { _meta: groupMeta, ...groupFields } = groupSchema;
                const basePath = `${section}.${groupKey}`;

                // Filter fields within the group if searching
                const visibleGroupFields = showAllFields
                    ? groupFields
                    : Object.fromEntries(
                        Object.entries(groupFields).filter(
                            ([fieldKey]) => fieldMatches(fieldKey, groupKey)
                        )
                    );

                return (
                    <ConfigFieldGroup
                        key={groupKey}
                        title={groupMeta?.label || groupKey}
                        fields={visibleGroupFields}
                        basePath={basePath}
                        config={config}
                        onFieldChange={onFieldChange}
                    />
                );
            })}
        </div>
    );
}
