import React from 'react';
import { ChevronUpIcon, ChevronDownIcon } from '@heroicons/react/24/outline';

export default function NumberField({ label, description, value, onChange, isModified, min, max, step = 1, unit }) {
    const handleInputChange = (e) => {
        const val = e.target.value === '' ? min || 0 : parseFloat(e.target.value);
        if (!isNaN(val)) {
            onChange(Math.min(max ?? val, Math.max(min ?? val, val)));
        }
    };

    const increment = () => {
        const newVal = parseFloat(value || 0) + parseFloat(step);
        const clamped = Math.min(max ?? newVal, newVal);
        // Round to avoid floating point issues
        onChange(Math.round(clamped * 1000) / 1000);
    };

    const decrement = () => {
        const newVal = parseFloat(value || 0) - parseFloat(step);
        const clamped = Math.max(min ?? newVal, newVal);
        // Round to avoid floating point issues
        onChange(Math.round(clamped * 1000) / 1000);
    };

    const isAtMin = min !== undefined && parseFloat(value) <= min;
    const isAtMax = max !== undefined && parseFloat(value) >= max;

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
                <div className="relative inline-flex">
                    <input
                        type="text"
                        inputMode="decimal"
                        value={value}
                        onChange={handleInputChange}
                        className="w-24 px-2 py-1.5 pr-7 text-sm bg-surface border border-border rounded text-text focus:outline-none focus:border-primary [appearance:textfield]"
                    />
                    <div className="absolute right-0 top-0 bottom-0 flex flex-col border-l border-border">
                        <button
                            type="button"
                            onClick={increment}
                            disabled={isAtMax}
                            className="flex-1 px-1 hover:bg-surface-hover disabled:opacity-30 disabled:cursor-not-allowed rounded-tr"
                            tabIndex={-1}
                        >
                            <ChevronUpIcon className="h-3 w-3 text-text-muted" />
                        </button>
                        <button
                            type="button"
                            onClick={decrement}
                            disabled={isAtMin}
                            className="flex-1 px-1 hover:bg-surface-hover disabled:opacity-30 disabled:cursor-not-allowed rounded-br border-t border-border"
                            tabIndex={-1}
                        >
                            <ChevronDownIcon className="h-3 w-3 text-text-muted" />
                        </button>
                    </div>
                </div>
                {unit && (
                    <span className="text-xs text-text-muted">{unit}</span>
                )}
            </div>
        </div>
    );
}
