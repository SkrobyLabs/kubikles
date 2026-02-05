import React, { useState, useRef, useEffect } from 'react';
import { ChevronDownIcon } from '@heroicons/react/24/outline';

// Source options for the metrics source selector
export const sourceOptions = [
    { value: 'auto', label: 'Auto' },
    { value: 'k8s', label: 'K8s Metrics API' },
    { value: 'prometheus', label: 'Prometheus' }
];

// Simple dropdown component matching SearchSelect styling
const SourceSelect = ({ value, onChange, options }) => {
    const [isOpen, setIsOpen] = useState(false);
    const wrapperRef = useRef(null);

    useEffect(() => {
        function handleClickOutside(event) {
            if (wrapperRef.current && !wrapperRef.current.contains(event.target)) {
                setIsOpen(false);
            }
        }
        document.addEventListener("mousedown", handleClickOutside);
        return () => document.removeEventListener("mousedown", handleClickOutside);
    }, []);

    const selectedOption = options.find(opt => opt.value === value) || options[0];

    const handleSelect = (optValue) => {
        onChange(optValue);
        setIsOpen(false);
    };

    return (
        <div className="relative" ref={wrapperRef}>
            <button
                onClick={() => setIsOpen(!isOpen)}
                className="flex items-center justify-between px-3 py-1.5 bg-surface border border-border rounded text-sm text-text hover:border-primary focus:outline-none focus:border-primary transition-colors min-w-[140px]"
            >
                <span className="truncate">{selectedOption.label}</span>
                <ChevronDownIcon className="h-4 w-4 text-gray-400 ml-2 shrink-0" />
            </button>

            {isOpen && (
                <div className="absolute z-50 w-full mt-1 bg-surface border border-border rounded shadow-lg">
                    {options.map((option) => (
                        <div
                            key={option.value}
                            className={`px-3 py-2 text-sm cursor-pointer hover:bg-primary/10 ${
                                option.value === value ? 'text-primary font-medium' : 'text-text'
                            }`}
                            onClick={() => handleSelect(option.value)}
                        >
                            {option.label}
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
};

export default SourceSelect;
