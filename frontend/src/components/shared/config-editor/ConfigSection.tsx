import React, { useEffect, useState, useCallback } from 'react';
import ConfigField from './fields/ConfigField';
import ConfigFieldGroup from './ConfigFieldGroup';
import { configSchema, isModified } from '~/config/configSchema';
import { GetCrashLogPath, OpenCrashLogDir, GetIssueRulesDir, OpenIssueRulesDir } from 'wailsjs/go/main/App';
import { useTheme } from '~/context';

// Async value sources
const asyncSources = {
    crashLogPath: GetCrashLogPath,
    issueRulesDir: GetIssueRulesDir,
};

// Action handlers
const actionHandlers = {
    openCrashLogDir: OpenCrashLogDir,
    openIssueRulesDir: OpenIssueRulesDir,
};

// Zoom constants (must match App.jsx)
const ZOOM_STORAGE_KEY = 'kubikles-zoom-level';
const ZOOM_DEFAULT = 1.0;

export default function ConfigSection({ section, config, onFieldChange, searchResults }: { section: string; config: any; onFieldChange: any; searchResults: any }) {
    const sectionSchema = (configSchema as Record<string, any>)[section];
    const [asyncValues, setAsyncValues] = useState<Record<string, any>>({});
    const { currentTheme, themes, switchTheme } = useTheme();

    // Zoom level state (reads from localStorage, updates body style)
    const [zoomLevel, setZoomLevelState] = useState(() => {
        const saved = localStorage.getItem(ZOOM_STORAGE_KEY);
        return saved ? parseFloat(saved) : ZOOM_DEFAULT;
    });

    const setZoomLevel = useCallback((value: any) => {
        const numValue = parseFloat(value);
        setZoomLevelState(numValue);
        (document.body.style as any).zoom = numValue;
        localStorage.setItem(ZOOM_STORAGE_KEY, numValue.toString());
    }, []);

    // Listen for zoom changes from scroll/other sources
    useEffect(() => {
        const handleZoomChanged = (e: any) => {
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
            const newValues: Record<string, any> = {};

            for (const [key, schema] of Object.entries(fields) as [string, any][]) {
                if (schema.asyncSource && (asyncSources as Record<string, any>)[schema.asyncSource]) {
                    try {
                        newValues[key] = await (asyncSources as Record<string, any>)[schema.asyncSource]();
                    } catch (err: any) {
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

    const handleAction = useCallback((action: string) => {
        if ((actionHandlers as Record<string, any>)[action]) {
            (actionHandlers as Record<string, any>)[action]().catch((err: any) => {
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
    const fieldMatches = (fieldKey: string, groupKey: string | null = null) => {
        if (showAllFields) return true;
        if (!matchingFields) return false;
        const fullKey = groupKey ? `${groupKey}.${fieldKey}` : fieldKey;
        return matchingFields.includes(fullKey);
    };

    // Check if any field in a group matches
    const groupHasMatches = (groupKey: string, groupFields: Record<string, any>) => {
        if (showAllFields) return true;
        if (!matchingFields) return false;
        return Object.keys(groupFields).some((fieldKey: any) =>
            matchingFields.includes(`${groupKey}.${fieldKey}`)
        );
    };

    // Separate top-level fields from nested groups
    const topLevelFields: Record<string, any> = {};
    const nestedGroups: Record<string, any> = {};

    Object.entries(fields).forEach(([key, schema]: [string, any]) => {
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
        ([groupKey, groupSchema]: [string, any]) => {
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
                    {visibleTopLevelFields.map(([key, schema]: [string, any]) => {
                        const path = `${section}.${key}`;

                        // Handle theme-sourced fields (e.g., theme selector)
                        if (schema.source === 'theme') {
                            const value = (themeValues as Record<string, any>)[key];
                            const setter = (themeSetters as Record<string, any>)[key];
                            const options = (themeOptions as Record<string, any>)[schema.optionsSource]?.map((t: any) => ({
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
                                    asyncValue={undefined}
                                    onAction={handleAction}
                                />
                            );
                        }

                        // Handle zoom-sourced fields (e.g., zoom level)
                        if (schema.source === 'zoom') {
                            const value = (zoomValues as Record<string, any>)[key];
                            const setter = (zoomSetters as Record<string, any>)[key];

                            return (
                                <ConfigField
                                    key={key}
                                    schema={schema}
                                    value={value}
                                    onChange={setter}
                                    isModified={value !== schema.default}
                                    asyncValue={undefined}
                                    onAction={handleAction}
                                />
                            );
                        }

                        const value = config?.[section]?.[key];

                        return (
                            <ConfigField
                                key={key}
                                schema={schema}
                                value={value}
                                onChange={(val: any) => onFieldChange(path, val)}
                                isModified={isModified(path, value)}
                                asyncValue={asyncValues[key]}
                                onAction={handleAction}
                            />
                        );
                    })}
                </div>
            )}

            {/* Nested groups */}
            {visibleNestedGroups.map(([groupKey, groupSchema]: [string, any]) => {
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
