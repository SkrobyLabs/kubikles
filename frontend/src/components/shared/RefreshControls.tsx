import React, { useEffect, useId, useRef, useState } from 'react';
import { ArrowPathIcon, CheckIcon, ChevronDownIcon, EyeIcon } from '@heroicons/react/24/outline';
import { useK8s, useNotification } from '~/context';
import { POLLING_INTERVALS } from '~/utils/streamingCompatibility';

export default function RefreshControls() {
    const { currentContext, connectionMode, setConnectionMode, pollingInterval, setPollingInterval,
        refreshContextsIfChanged, refreshNamespaces, triggerRefresh } = useK8s();
    const { addNotification } = useNotification();
    const [refreshing, setRefreshing] = useState(false);
    const [isOpen, setIsOpen] = useState(false);
    const dropdownRef = useRef<HTMLDivElement>(null);
    const triggerRef = useRef<HTMLButtonElement>(null);
    const menuRef = useRef<HTMLDivElement>(null);
    const menuId = useId();
    const selected = connectionMode === 'polling' ? String(pollingInterval / 1000) : connectionMode;
    const modeLabel = connectionMode === 'streaming' ? 'Watcher' : connectionMode === 'manual' ? 'Automatic refresh disabled' : `Refresh every ${pollingInterval / 1000} seconds`;
    const options = [
        { value: 'streaming', label: 'Watcher' },
        { value: 'manual', label: 'Disabled' },
        ...POLLING_INTERVALS.map(seconds => ({ value: String(seconds), label: `${seconds}s` })),
    ];

    useEffect(() => { setIsOpen(false); }, [currentContext]);
    useEffect(() => {
        if (!isOpen) return;
        menuRef.current?.querySelector<HTMLButtonElement>('[aria-checked="true"]')?.focus();
        const closeOutside = (event: MouseEvent) => {
            if (!dropdownRef.current?.contains(event.target as Node)) setIsOpen(false);
        };
        document.addEventListener('mousedown', closeOutside);
        return () => document.removeEventListener('mousedown', closeOutside);
    }, [isOpen]);
    if (!currentContext) return null;

    const refresh = async () => {
        setRefreshing(true);
        triggerRefresh();
        try {
            await Promise.all([refreshContextsIfChanged(), refreshNamespaces()]);
        } catch (error) {
            addNotification({ type: 'error', title: 'Refresh failed', message: String(error) });
        } finally {
            setRefreshing(false);
        }
    };

    return (
        <div ref={dropdownRef} className="relative no-drag flex h-8 items-center rounded-md border border-border text-xs"
            onBlur={event => {
                if (!event.currentTarget.contains(event.relatedTarget as Node)) setIsOpen(false);
            }}
            onKeyDown={event => {
                if (event.key === 'Escape' && isOpen) {
                    event.preventDefault();
                    event.stopPropagation();
                    setIsOpen(false);
                    triggerRef.current?.focus();
                }
            }}>
            <button type="button" onClick={refresh} disabled={refreshing} title="Refresh now" aria-label="Refresh now"
                className="h-full rounded-l-md px-2 text-gray-400 hover:bg-white/5 hover:text-text transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary disabled:opacity-50">
                <ArrowPathIcon className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />
            </button>
            <button ref={triggerRef} type="button" aria-label={`Resource refresh mode: ${modeLabel}`} title={modeLabel}
                aria-haspopup="menu" aria-expanded={isOpen} aria-controls={isOpen ? menuId : undefined}
                onClick={() => setIsOpen(open => !open)}
                onKeyDown={event => {
                    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                        event.preventDefault();
                        setIsOpen(true);
                    }
                }}
                className={`flex h-full min-w-[3rem] items-center justify-center gap-1 rounded-r-md border-l border-border px-1.5 hover:bg-white/5 transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-primary ${connectionMode === 'polling' ? 'text-amber-400' : 'text-gray-400 hover:text-text'}`}>
                {connectionMode === 'streaming' ? <EyeIcon className="h-4 w-4" /> : <span className="tabular-nums">{connectionMode === 'manual' ? '--' : `${pollingInterval / 1000}s`}</span>}
                <ChevronDownIcon className={`h-3 w-3 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
            </button>
            {isOpen && (
                <div ref={menuRef} id={menuId} role="menu" aria-label="Resource refresh mode"
                    className="absolute right-0 top-full z-50 mt-1 w-40 rounded-lg border border-border bg-surface py-1 shadow-xl"
                    onKeyDown={event => {
                        const items = Array.from(event.currentTarget.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]'));
                        const index = items.indexOf(document.activeElement as HTMLButtonElement);
                        let next: number;
                        if (event.key === 'ArrowDown') next = (index + 1) % items.length;
                        else if (event.key === 'ArrowUp') next = (index - 1 + items.length) % items.length;
                        else if (event.key === 'Home') next = 0;
                        else if (event.key === 'End') next = items.length - 1;
                        else return;
                        event.preventDefault();
                        items[next]?.focus();
                    }}>
                    {options.map(option => (
                        <button key={option.value} type="button" role="menuitemradio" aria-checked={selected === option.value} tabIndex={-1}
                            onClick={() => {
                                if (option.value === 'streaming' || option.value === 'manual') setConnectionMode(option.value);
                                else {
                                    setPollingInterval(Number(option.value) * 1000);
                                    setConnectionMode('polling');
                                }
                                setIsOpen(false);
                                triggerRef.current?.focus();
                            }}
                            className={`flex w-full items-center gap-2 px-3 py-2 text-left text-xs transition-colors hover:bg-white/5 focus:bg-white/5 focus:outline-none ${selected === option.value ? 'bg-primary/10 text-primary' : 'text-gray-300'}`}>
                            <span className="flex h-4 w-4 items-center justify-center text-gray-500">
                                {option.value === 'streaming' ? <EyeIcon className="h-4 w-4" /> : option.value === 'manual' ? '--' : null}
                            </span>
                            <span className="flex-1">{option.label}</span>
                            {selected === option.value && <CheckIcon className="h-3.5 w-3.5" />}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}
