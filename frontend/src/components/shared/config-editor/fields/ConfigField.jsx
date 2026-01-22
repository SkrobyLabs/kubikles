import React from 'react';
import BooleanField from './BooleanField';
import NumberField from './NumberField';
import EnumField from './EnumField';
import TextField from './TextField';

export default function ConfigField({ schema, value, onChange, isModified }) {
    const { type, label, description } = schema;

    switch (type) {
        case 'boolean':
            return (
                <BooleanField
                    label={label}
                    description={description}
                    value={value}
                    onChange={onChange}
                    isModified={isModified}
                />
            );

        case 'number':
            return (
                <NumberField
                    label={label}
                    description={description}
                    value={value}
                    onChange={onChange}
                    isModified={isModified}
                    min={schema.min}
                    max={schema.max}
                    step={schema.step}
                    unit={schema.unit}
                />
            );

        case 'enum':
            return (
                <EnumField
                    label={label}
                    description={description}
                    value={value}
                    onChange={onChange}
                    isModified={isModified}
                    options={schema.options}
                />
            );

        case 'string':
        default:
            return (
                <TextField
                    label={label}
                    description={description}
                    value={value}
                    onChange={onChange}
                    isModified={isModified}
                    placeholder={schema.placeholder}
                />
            );
    }
}
