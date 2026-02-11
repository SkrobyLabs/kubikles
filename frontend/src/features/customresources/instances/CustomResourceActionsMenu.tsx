import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { PencilSquareIcon, TrashIcon, EllipsisVerticalIcon, InformationCircleIcon } from '@heroicons/react/24/outline';

export default function CustomResourceActionsMenu({ resource, isOpen, menuPosition, onOpenChange, onShowDetails, onEditYaml, onDelete }: any) {
    const buttonRef = useRef<any>(null);
    const menuRef = useRef<any>(null);

    useEffect(() => {
        if (!isOpen) return;

        const handleClickOutside = (event: any) => {
            if (buttonRef.current && !buttonRef.current.contains(event.target)) {
                if (menuRef.current && !menuRef.current.contains(event.target)) {
                    onOpenChange(false);
                }
            }
        };

        const handleScroll = () => {
            if (isOpen) onOpenChange(false);
        };

        document.addEventListener('mousedown', handleClickOutside);
        window.addEventListener('scroll', handleScroll, true);

        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
            window.removeEventListener('scroll', handleScroll, true);
        };
    }, [isOpen, onOpenChange]);

    const toggleMenu = (e: any) => {
        e.stopPropagation();
        onOpenChange(!isOpen, buttonRef.current);
    };

    const handleAction = (action: any) => {
        onOpenChange(false);
        action();
    };

    const menu = (
        <div
            ref={menuRef}
            className="w-48 bg-surface-light border border-border rounded-md shadow-lg py-1"
            style={{ position: 'fixed', top: `${menuPosition.top}px`, left: `${menuPosition.left}px`, zIndex: 99999 }}
            onClick={(e) => e.stopPropagation()}
        >
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onShowDetails(resource)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <InformationCircleIcon className="h-4 w-4" />
                View Details
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onEditYaml(resource)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <PencilSquareIcon className="h-4 w-4" />
                Edit YAML
            </button>
            <div className="h-px bg-surface-hover my-1" />
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onDelete(resource)); }}
                className="w-full text-left px-4 py-2 text-sm text-red-400 hover:bg-surface-hover flex items-center gap-2"
            >
                <TrashIcon className="h-4 w-4" />
                Delete
            </button>
        </div>
    );

    return (
        <>
            <button
                ref={buttonRef}
                onClick={toggleMenu}
                className="p-1 rounded-full hover:bg-white/10 text-gray-400 hover:text-white transition-colors"
            >
                <EllipsisVerticalIcon className="h-5 w-5" />
            </button>
            {isOpen && createPortal(menu, document.body)}
        </>
    );
}
