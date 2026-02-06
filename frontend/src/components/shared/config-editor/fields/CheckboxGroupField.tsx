import React, { useState } from 'react';
import { CheckIcon, XMarkIcon } from '@heroicons/react/24/outline';

export default function CheckboxGroupField({ label, description, value, onChange, isModified, options }: { label: any; description: any; value: any; onChange: any; isModified: any; options: any }) {
    const [customInput, setCustomInput] = useState('');
    const selected = value || [];
    const knownValues = new Set(options.map((o: any) => o.value));
    const customEntries = selected.filter((v: any) => !knownValues.has(v));

    const toggle = (optValue: any) => {
        const updated = selected.includes(optValue)
            ? selected.filter((v: any) => v !== optValue)
            : [...selected, optValue];
        onChange(updated);
    };

    const addCustom = () => {
        const trimmed = customInput.trim();
        if (!trimmed || selected.includes(trimmed)) return;
        onChange([...selected, trimmed]);
        setCustomInput('');
    };

    const removeCustom = (entry: any) => {
        onChange(selected.filter((v: any) => v !== entry));
    };

    const handleKeyDown = (e: any) => {
        if (e.key === 'Enter') {
            e.preventDefault();
            addCustom();
        }
    };

    return (
        <div className="py-2">
            <div className="text-sm font-medium text-text mb-1">
                {label}
                {isModified && <span className="ml-2 text-xs text-primary">(modified)</span>}
            </div>
            {description && (
                <div className="text-sm text-gray-300 mb-2">{description}</div>
            )}

            {/* Checkbox list for known tools */}
            <div className="space-y-1.5 mb-3">
                {options.map((opt: any, idx: number) => {
                    const checked = selected.includes(opt.value);
                    const prevOpt = idx > 0 ? options[idx - 1] : null;
                    const showDivider = opt.warn && (!prevOpt || !prevOpt.warn);
                    return (
                        <React.Fragment key={opt.value}>
                            {showDivider && (
                                <div className="border-t border-border pt-2 mt-2">
                                    <span className="text-xs text-amber-400/80 font-medium">Dangerous — disabled by default</span>
                                </div>
                            )}
                            <div
                                className="flex items-center gap-2 cursor-pointer"
                                onClick={() => toggle(opt.value)}
                            >
                                <button
                                    type="button"
                                    className={`w-4 h-4 rounded border flex items-center justify-center transition-colors shrink-0 ${
                                        checked
                                            ? opt.warn ? 'border-amber-500 bg-amber-500 hover:bg-amber-500/90' : 'border-primary bg-primary hover:bg-primary/90'
                                            : 'border-gray-500 bg-transparent hover:border-gray-400'
                                    }`}
                                >
                                    {checked && <CheckIcon className="w-3 h-3 text-white" />}
                                </button>
                                <span className={`text-sm ${opt.warn && checked ? 'text-amber-400' : 'text-gray-300'}`}>{opt.label}</span>
                            </div>
                        </React.Fragment>
                    );
                })}
            </div>

            {/* Custom tool tags */}
            {customEntries.length > 0 && (
                <div className="flex flex-wrap gap-1.5 mb-2">
                    {customEntries.map((entry: any) => (
                        <span
                            key={entry}
                            className="inline-flex items-center gap-1 px-2 py-0.5 rounded bg-white/10 text-xs text-gray-300 font-mono"
                        >
                            {entry}
                            <button
                                onClick={() => removeCustom(entry)}
                                className="text-gray-500 hover:text-red-400 transition-colors"
                            >
                                <XMarkIcon className="w-3 h-3" />
                            </button>
                        </span>
                    ))}
                </div>
            )}

            {/* Free-text input for external tools */}
            <div className="flex items-center gap-2">
                <input
                    type="text"
                    value={customInput}
                    onChange={(e: any) => setCustomInput(e.target.value)}
                    onKeyDown={handleKeyDown}
                    placeholder="mcp__external__tool_name"
                    className="flex-1 bg-background border border-border rounded px-2 py-1 text-xs text-text placeholder-gray-500 outline-none focus:border-primary font-mono"
                />
                <button
                    onClick={addCustom}
                    disabled={!customInput.trim()}
                    className="px-2 py-1 text-xs rounded bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-30 disabled:cursor-not-allowed"
                >
                    Add
                </button>
            </div>
        </div>
    );
}
