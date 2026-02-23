import React, { useState, useRef, useEffect, useMemo } from 'react';
import { ChevronDownIcon } from '@heroicons/react/24/outline';
import { GetAIModels } from '~/lib/wailsjs-adapter/go/main/App';

interface AIModelOption {
    value: string;
    label: string;
    provider: string;
    providerLabel: string;
    available: boolean;
}

interface Props {
    label: string;
    description?: string;
    value: string;
    onChange: (value: string) => void;
    isModified: boolean;
}

export default function ModelSelectField({ label, description, value, onChange, isModified }: Props) {
    const [isOpen, setIsOpen] = useState(false);
    const [models, setModels] = useState<AIModelOption[]>([]);
    const [customInput, setCustomInput] = useState('');
    const [showCustom, setShowCustom] = useState(false);
    const wrapperRef = useRef<HTMLDivElement>(null);
    const inputRef = useRef<HTMLInputElement>(null);

    useEffect(() => {
        GetAIModels().then((result: AIModelOption[]) => {
            if (result) setModels(result);
        }).catch(() => {});
    }, []);

    useEffect(() => {
        function handleClickOutside(event: MouseEvent) {
            if (wrapperRef.current && !wrapperRef.current.contains(event.target as Node)) {
                setIsOpen(false);
                setShowCustom(false);
            }
        }
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, []);

    // Group models by provider
    const grouped = useMemo(() => {
        const groups: Record<string, { label: string; available: boolean; models: AIModelOption[] }> = {};
        for (const m of models) {
            if (!groups[m.provider]) {
                groups[m.provider] = { label: m.providerLabel, available: m.available, models: [] };
            }
            groups[m.provider].models.push(m);
        }
        return groups;
    }, [models]);

    // Find selected option label
    const selectedOption = models.find(m => m.value === value);
    const displayLabel = selectedOption
        ? `${selectedOption.providerLabel} / ${selectedOption.label}`
        : value || 'Select model';

    const handleCustomSubmit = () => {
        const trimmed = customInput.trim();
        if (trimmed) {
            onChange(trimmed);
            setCustomInput('');
            setShowCustom(false);
            setIsOpen(false);
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
            <div className="relative inline-block" ref={wrapperRef}>
                <button
                    onClick={() => setIsOpen(!isOpen)}
                    className="flex items-center justify-between gap-2 px-3 py-1.5 text-sm bg-surface border border-border rounded text-text hover:border-primary focus:outline-none focus:border-primary min-w-[220px]"
                >
                    <span className="truncate">{displayLabel}</span>
                    <ChevronDownIcon className={`h-4 w-4 text-gray-400 transition-transform flex-shrink-0 ${isOpen ? 'rotate-180' : ''}`} />
                </button>
                {isOpen && (
                    <div className="absolute left-0 z-50 mt-1 min-w-full bg-surface border border-border rounded shadow-lg py-1 max-h-[300px] overflow-y-auto">
                        {Object.entries(grouped).map(([provID, group]) => (
                            <div key={provID}>
                                <div className={`px-3 py-1.5 text-[10px] font-semibold uppercase tracking-wider ${group.available ? 'text-text-muted' : 'text-text-muted/50'}`}>
                                    {group.label}
                                    {!group.available && <span className="ml-1 text-[9px] normal-case">(not available)</span>}
                                </div>
                                {group.models.map(m => (
                                    <button
                                        key={m.value}
                                        onClick={() => {
                                            onChange(m.value);
                                            setIsOpen(false);
                                        }}
                                        className={`w-full text-left px-3 py-1.5 pl-6 text-sm hover:bg-primary/10 whitespace-nowrap ${
                                            m.value === value
                                                ? 'text-primary font-medium'
                                                : group.available
                                                    ? 'text-text'
                                                    : 'text-text-muted/50'
                                        }`}
                                    >
                                        {m.label}
                                    </button>
                                ))}
                            </div>
                        ))}
                        <div className="border-t border-border mt-1 pt-1">
                            {!showCustom ? (
                                <button
                                    onClick={() => {
                                        setShowCustom(true);
                                        setTimeout(() => inputRef.current?.focus(), 0);
                                    }}
                                    className="w-full text-left px-3 py-1.5 text-sm text-text-muted hover:bg-primary/10"
                                >
                                    Custom model...
                                </button>
                            ) : (
                                <div className="px-3 py-1.5 flex gap-1">
                                    <input
                                        ref={inputRef}
                                        type="text"
                                        value={customInput}
                                        onChange={e => setCustomInput(e.target.value)}
                                        onKeyDown={e => {
                                            if (e.key === 'Enter') handleCustomSubmit();
                                            if (e.key === 'Escape') { setShowCustom(false); setCustomInput(''); }
                                        }}
                                        placeholder="provider/model"
                                        className="flex-1 px-2 py-1 text-sm bg-background border border-border rounded text-text focus:outline-none focus:border-primary min-w-0"
                                    />
                                    <button
                                        onClick={handleCustomSubmit}
                                        disabled={!customInput.trim()}
                                        className="px-2 py-1 text-xs bg-primary text-white rounded disabled:opacity-40"
                                    >
                                        Set
                                    </button>
                                </div>
                            )}
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
