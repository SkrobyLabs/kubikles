import React from 'react';
import ConfigField from './fields/ConfigField';
import { isModified } from '../../../config/configSchema';

export default function ConfigFieldGroup({ title, fields, basePath, config, onFieldChange }) {
    return (
        <div className="border border-border rounded-lg p-4 space-y-4">
            <h4 className="text-sm font-semibold text-text">{title}</h4>
            {Object.entries(fields).map(([key, schema]) => {
                // Skip _meta entries
                if (key === '_meta') return null;
                // Skip nested groups (handled separately)
                if (schema._meta?.isNested) return null;

                const path = `${basePath}.${key}`;
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
