import React from 'react';

export default function TextField({ label, description, value, onChange, isModified, placeholder }) {
    return (
        <div className="py-2">
            <div className="text-sm font-medium text-text mb-1">
                {label}
                {isModified && <span className="ml-2 text-xs text-primary">(modified)</span>}
            </div>
            {description && (
                <p className="text-xs text-text-muted mb-2">{description}</p>
            )}
            <input
                type="text"
                value={value ?? ''}
                onChange={(e) => onChange(e.target.value)}
                placeholder={placeholder}
                className="w-64 px-2 py-1.5 text-sm bg-surface border border-border rounded text-text focus:outline-none focus:border-primary"
            />
        </div>
    );
}
