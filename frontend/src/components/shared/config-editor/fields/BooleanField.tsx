import React from 'react';
import { CheckIcon } from '@heroicons/react/24/outline';

export default function BooleanField({ label, description, subtext, value, onChange, isModified }) {
    return (
        <div className="py-2">
            <div className="text-sm font-medium text-text mb-1">
                {label}
                {isModified && <span className="ml-2 text-xs text-primary">(modified)</span>}
            </div>
            <div
                className="flex items-start gap-2 cursor-pointer"
                onClick={() => onChange(!value)}
            >
                <button
                    type="button"
                    className={`w-4 h-4 mt-0.5 rounded border flex items-center justify-center transition-colors shrink-0 ${
                        value
                            ? 'border-primary bg-primary hover:bg-primary/90'
                            : 'border-gray-500 bg-transparent hover:border-gray-400'
                    }`}
                >
                    {value && <CheckIcon className="w-3 h-3 text-white" />}
                </button>
                <div className="flex flex-col">
                    {description && (
                        <span className="text-sm text-gray-300">{description}</span>
                    )}
                    {subtext && (
                        <span className="text-xs text-gray-500 mt-0.5">{subtext}</span>
                    )}
                </div>
            </div>
        </div>
    );
}
