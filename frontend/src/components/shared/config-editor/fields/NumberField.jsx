import React from 'react';

export default function NumberField({ label, description, value, onChange, isModified, min, max, step, unit }) {
    const handleInputChange = (e) => {
        const val = e.target.value === '' ? min || 0 : parseInt(e.target.value, 10);
        if (!isNaN(val)) {
            onChange(Math.min(max ?? val, Math.max(min ?? val, val)));
        }
    };

    return (
        <div className="py-2">
            <div className="text-sm font-medium text-text mb-1">
                {label}
                {isModified && <span className="ml-2 text-xs text-primary">(modified)</span>}
            </div>
            {description && (
                <p className="text-xs text-text-muted mb-2">{description}</p>
            )}
            <div className="flex items-center gap-2">
                <input
                    type="number"
                    value={value}
                    onChange={handleInputChange}
                    min={min}
                    max={max}
                    step={step}
                    className="w-24 px-2 py-1.5 text-sm bg-surface border border-border rounded text-text focus:outline-none focus:border-primary"
                />
                {unit && (
                    <span className="text-xs text-text-muted">{unit}</span>
                )}
            </div>
        </div>
    );
}
