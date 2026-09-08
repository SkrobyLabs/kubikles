import React, { useEffect, useRef } from 'react';
import type { OperatorAction } from './types';

export default function OperatorActionsDropdown({ actions, disabled, onAction }: {
    actions: OperatorAction[]; disabled: boolean; onAction: (action: OperatorAction) => void;
}) {
    const ref = useRef<HTMLDetailsElement>(null);
    useEffect(() => {
        const dismiss = (event: MouseEvent) => {
            if (ref.current && !ref.current.contains(event.target as Node)) ref.current.open = false;
        };
        const escape = (event: KeyboardEvent) => {
            if (event.key === 'Escape' && ref.current?.open) {
                ref.current.open = false;
                ref.current.querySelector('summary')?.focus();
            }
        };
        document.addEventListener('mousedown', dismiss);
        document.addEventListener('keydown', escape);
        return () => {
            document.removeEventListener('mousedown', dismiss);
            document.removeEventListener('keydown', escape);
        };
    }, []);
    useEffect(() => { if (disabled && ref.current) ref.current.open = false; }, [disabled]);
    if (!actions.length) return null;
    return <details ref={ref} className="relative shrink-0" onBlur={event => {
        if (event.relatedTarget && !event.currentTarget.contains(event.relatedTarget as Node)) event.currentTarget.open = false;
    }}>
        <summary aria-disabled={disabled} onClick={event => { if (disabled) event.preventDefault(); }}
            className={`px-2 py-1 text-xs text-gray-300 hover:bg-surface-hover rounded cursor-pointer ${disabled ? 'opacity-50' : ''}`}>
            Operator actions
        </summary>
        <div className="absolute right-0 top-full mt-1 w-60 z-50 py-1 bg-surface-light border border-border rounded shadow-lg">
            {actions.map(action => <button key={action.id} disabled={disabled} onClick={() => {
                if (ref.current) ref.current.open = false;
                onAction(action);
            }} className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-surface-hover disabled:opacity-50">{action.label}</button>)}
        </div>
    </details>;
}
