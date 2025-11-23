import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { EllipsisVerticalIcon } from '@heroicons/react/24/outline';

export default function StatefulSetActionsMenu({ statefulSet, isOpen, onOpenChange, onEditYaml, onRestart, onDelete }) {
    const [position, setPosition] = useState({ top: 0, left: 0 });
    const buttonRef = useRef(null);

    useEffect(() => {
        const handleClickOutside = (event) => {
            if (buttonRef.current && !buttonRef.current.contains(event.target)) {
                // Check if click is inside the menu (which is now in a portal)
                const menu = document.getElementById(`statefulset-menu-${statefulSet.metadata.uid}`);
                if (menu && !menu.contains(event.target)) {
                    onOpenChange(false);
                }
            }
        };

        const handleScroll = () => {
            if (isOpen) onOpenChange(false);
        };

        document.addEventListener('mousedown', handleClickOutside);
        window.addEventListener('scroll', handleScroll, true); // Capture scroll events
        window.addEventListener('resize', handleScroll);

        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
            window.removeEventListener('scroll', handleScroll, true);
            window.removeEventListener('resize', handleScroll);
        };
    }, [isOpen, statefulSet.metadata.uid]);

    const toggleMenu = (e) => {
        e.stopPropagation();
        if (!isOpen) {
            const rect = buttonRef.current.getBoundingClientRect();
            // Align right edge of menu with right edge of button
            // w-48 is 12rem = 192px
            setPosition({
                top: rect.bottom + window.scrollY,
                left: rect.right - 192 + window.scrollX
            });
        }
        onOpenChange(!isOpen);
    };

    const handleAction = (action) => {
        onOpenChange(false);
        action();
    };

    const menu = (
        <div
            id={`statefulset-menu-${statefulSet.metadata.uid}`}
            className="fixed w-48 bg-background border border-border rounded-md shadow-lg z-50 py-1"
            style={{ top: position.top, left: position.left }}
            onClick={(e) => e.stopPropagation()}
        >
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onEditYaml(statefulSet)); }}
                className="w-full text-left px-4 py-2 text-sm text-text hover:bg-white/5 transition-colors"
            >
                Edit YAML
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onRestart(statefulSet)); }}
                className="w-full text-left px-4 py-2 text-sm text-text hover:bg-white/5 transition-colors"
            >
                Restart
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onDelete(statefulSet)); }}
                className="w-full text-left px-4 py-2 text-sm text-red-400 hover:bg-white/5 transition-colors"
            >
                Delete
            </button>
        </div >
    );

    return (
        <>
            <button
                ref={buttonRef}
                onClick={toggleMenu}
                className="p-1 rounded-full hover:bg-surface-hover text-gray-400 hover:text-text transition-colors"
            >
                <EllipsisVerticalIcon className="h-5 w-5" />
            </button>
            {isOpen && createPortal(menu, document.body)}
        </>
    );
}
