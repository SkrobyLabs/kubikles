import React from 'react';
import ConfigField from './fields/ConfigField';
import { isModified } from '../../../config/configSchema';
import { useTheme } from '../../../context/ThemeContext';

export default function ConfigFieldGroup({ title, fields, basePath, config, onFieldChange }) {
    const { uiFont, monoFont, setUiFont, setMonoFont, uiFonts, monoFonts } = useTheme();

    // Map for theme-sourced values and setters
    const themeValues = { uiFont, monoFont };
    const themeSetters = { uiFont: setUiFont, monoFont: setMonoFont };
    const themeOptions = { uiFonts, monoFonts };

    return (
        <div className="border border-border rounded-lg p-4 space-y-4">
            <h4 className="text-sm font-semibold text-text">{title}</h4>
            {Object.entries(fields).map(([key, schema]) => {
                // Skip _meta entries
                if (key === '_meta') return null;
                // Skip nested groups (handled separately)
                if (schema._meta?.isNested) return null;

                const path = `${basePath}.${key}`;

                // Handle theme-sourced fields (fonts)
                if (schema.source === 'theme') {
                    const value = themeValues[key];
                    const setter = themeSetters[key];
                    const options = themeOptions[schema.optionsSource]?.map(f => ({
                        value: f.id,
                        label: f.name
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

                const value = path.split('.').reduce((obj, k) => obj?.[k], config);

                return (
                    <ConfigField
                        key={key}
                        schema={schema}
                        value={value}
                        onChange={(val) => onFieldChange(path, val)}
                        isModified={isModified(path, value)}
                    />
                );
            })}
        </div>
    );
}
