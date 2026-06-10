import React, { useMemo, useRef, useEffect } from 'react';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { tokenizeSearchHighlights } from '~/utils/search/highlightTokens';

// Self-contained presentational pieces used by ResourceList:
// the search-highlight overlay, tri-state/row checkboxes, and the DnD sortable header.

export const SearchHighlightOverlay = React.memo(({ query }: { query: string }) => {
    const tokens = useMemo(() => tokenizeSearchHighlights(query), [query]);
    if (!query) return null;

    return (
        <div
            aria-hidden="true"
            className="pointer-events-none absolute inset-0 z-0 overflow-hidden whitespace-pre rounded-md border border-transparent pl-9 pr-4 py-1.5 text-sm leading-5 text-text"
        >
            {tokens.map((token, index) => (
                <span
                    key={`${index}-${token.text}`}
                    className={
                        token.kind === 'regex'
                            ? 'font-semibold text-primary'
                            : token.kind === 'invalidRegex'
                                ? 'font-semibold text-error'
                                : undefined
                    }
                >
                    {token.text}
                </span>
            ))}
        </div>
    );
});

// Tri-state checkbox props
export interface TriStateCheckboxProps {
    state: 'none' | 'some' | 'all';
    onChange: () => void;
    disabled?: boolean;
}

// Tri-state checkbox component for header (memoized to prevent re-renders)
export const TriStateCheckbox = React.memo(({ state, onChange, disabled = false }: TriStateCheckboxProps) => {
    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        e.stopPropagation();
        if (!disabled) onChange();
    };

    return (
        <input
            type="checkbox"
            checked={state === 'all'}
            ref={(el) => { if (el) el.indeterminate = state === 'some'; }}
            onChange={handleChange}
            disabled={disabled}
            className={disabled ? 'opacity-50 cursor-not-allowed' : ''}
        />
    );
});

// Row checkbox props
export interface RowCheckboxProps {
    checked: boolean;
    onChange: (e: React.ChangeEvent<HTMLInputElement>) => void;
    disabled?: boolean;
}

// Row checkbox component (memoized to prevent re-renders on scroll)
export const RowCheckbox = React.memo(({ checked, onChange, disabled = false }: RowCheckboxProps) => {
    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        e.stopPropagation();
        if (!disabled) onChange(e);
    };

    return (
        <input
            type="checkbox"
            checked={checked}
            onChange={handleChange}
            disabled={disabled}
            className={disabled ? 'opacity-50 cursor-not-allowed' : ''}
        />
    );
});

// Sortable header cell for column reordering via DnD
export interface SortableHeaderProps {
    id: string;
    disabled: boolean;
    children: React.ReactNode;
    className: string;
    style: React.CSSProperties;
    onClick?: () => void;
}

export const SortableHeader = ({ id, disabled, children, className, style, onClick }: SortableHeaderProps) => {
    const {
        attributes,
        listeners,
        setNodeRef,
        transform,
        transition,
        isDragging,
    } = useSortable({ id, disabled });

    const combinedStyle: React.CSSProperties = {
        ...style,
        transform: CSS.Translate.toString(transform),
        transition,
        opacity: isDragging ? 0.5 : undefined,
        zIndex: isDragging ? 30 : undefined,
    };

    return (
        <th
            ref={setNodeRef}
            className={className}
            style={combinedStyle}
            {...attributes}
            {...listeners}
            onClick={onClick}
        >
            {children}
        </th>
    );
};
