import React, { useState, useEffect, useRef } from 'react';
import { ChevronDownIcon, MagnifyingGlassIcon } from '@heroicons/react/24/outline';

export default function SearchSelect({ options, value, onChange, placeholder = "Select...", className = "" }) {
    const [isOpen, setIsOpen] = useState(false);
    const [searchTerm, setSearchTerm] = useState("");
    const wrapperRef = useRef(null);
    const inputRef = useRef(null);

    // Helper to get display label for an option
    const getDisplayLabel = (option) => {
        return option === '' ? 'All Namespaces' : option;
    };

    useEffect(() => {
        function handleClickOutside(event) {
            if (wrapperRef.current && !wrapperRef.current.contains(event.target)) {
                setIsOpen(false);
            }
        }
        document.addEventListener("mousedown", handleClickOutside);
        return () => {
            document.removeEventListener("mousedown", handleClickOutside);
        };
    }, [wrapperRef]);

    useEffect(() => {
        if (isOpen && inputRef.current) {
            inputRef.current.focus();
        }
        if (!isOpen) {
            setSearchTerm("");
        }
    }, [isOpen]);

    const filteredOptions = options.filter(option =>
        getDisplayLabel(option).toLowerCase().includes(searchTerm.toLowerCase())
    );

    return (
        <div className={`relative ${className}`} ref={wrapperRef}>
            <button
                onClick={() => setIsOpen(!isOpen)}
                className="w-full flex items-center justify-between px-3 py-2 bg-surface border border-border rounded text-sm text-text hover:border-primary focus:outline-none focus:border-primary transition-colors"
            >
                <span className="truncate">{value !== undefined && value !== null ? getDisplayLabel(value) : placeholder}</span>
                <ChevronDownIcon className="h-4 w-4 text-gray-400 ml-2 shrink-0" />
            </button>

            {isOpen && (
                <div className="absolute z-50 w-full mt-1 bg-surface border border-border rounded shadow-lg max-h-60 flex flex-col">
                    <div className="p-2 border-b border-border sticky top-0 bg-surface">
                        <div className="relative">
                            <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-2 top-1/2 transform -translate-y-1/2" />
                            <input
                                ref={inputRef}
                                type="text"
                                className="w-full bg-background border border-border rounded pl-8 pr-2 py-1 text-sm text-text focus:outline-none focus:border-primary"
                                placeholder="Search..."
                                value={searchTerm}
                                onChange={(e) => setSearchTerm(e.target.value)}
                                onClick={(e) => e.stopPropagation()}
                                autoComplete="off"
                                autoCorrect="off"
                                spellCheck="false"
                            />
                        </div>
                    </div>
                    <div className="overflow-y-auto flex-1">
                        {filteredOptions.length > 0 ? (
                            filteredOptions.map((option) => (
                                <div
                                    key={option || '__all__'}
                                    className={`px-3 py-2 text-sm cursor-pointer hover:bg-primary/10 ${option === value ? 'text-primary font-medium' : 'text-text'}`}
                                    onClick={() => {
                                        onChange(option);
                                        setIsOpen(false);
                                    }}
                                >
                                    {getDisplayLabel(option)}
                                </div>
                            ))
                        ) : (
                            <div className="px-3 py-2 text-sm text-gray-500 text-center">
                                No results found
                            </div>
                        )}
                    </div>
                </div>
            )}
        </div>
    );
}
