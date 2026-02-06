import React from 'react';
import BooleanField from './BooleanField';
import NumberField from './NumberField';
import EnumField from './EnumField';
import TextField from './TextField';
import ReadonlyField from './ReadonlyField';
import CheckboxGroupField from './CheckboxGroupField';
import SidebarLayoutEditor from './SidebarLayoutEditor';

export default function ConfigField({ schema, value, onChange, isModified, asyncValue, onAction }) {
    const { type, label, description } = schema;

    switch (type) {
        case 'boolean':
            return (
                <BooleanField
                    label={label}
                    description={description}
                    subtext={schema.subtext}
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

        case 'checkboxGroup':
            return (
                <CheckboxGroupField
                    label={label}
                    description={description}
                    value={value}
                    onChange={onChange}
                    isModified={isModified}
                    options={schema.options}
                />
            );

        case 'sidebarLayout':
            return (
                <SidebarLayoutEditor
                    value={value}
                    onChange={onChange}
                    isModified={isModified}
                />
            );

        case 'readonly':
            return (
                <ReadonlyField
                    label={label}
                    description={description}
                    value={asyncValue ?? value}
                    showCopy={schema.showCopy !== false}
                    showOpenFolder={schema.showOpenFolder}
                    onOpenFolder={schema.action && onAction ? () => onAction(schema.action) : undefined}
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
