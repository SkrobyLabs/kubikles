import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { PencilSquareIcon, TrashIcon, EllipsisVerticalIcon, ShareIcon } from '@heroicons/react/24/outline';

export default function PVActionsMenu({ pv, isOpen, onOpenChange, onEditYaml, onShowDependencies, onDelete }) {
    const [position, setPosition] = useState({ top: 0, left: 0 });
    const buttonRef = useRef(null);

    useEffect(() => {
        const handleClickOutside = (event) => {
            if (buttonRef.current && !buttonRef.current.contains(event.target)) {
                const menu = document.getElementById(`pv-menu-${pv.metadata.uid}`);
                if (menu && !menu.contains(event.target)) {
                    onOpenChange(false);
                }
            }
        };

        const handleScroll = () => {
            if (isOpen) onOpenChange(false);
        };

        document.addEventListener('mousedown', handleClickOutside);
        window.addEventListener('scroll', handleScroll, true);
        window.addEventListener('resize', handleScroll);

        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
            window.removeEventListener('scroll', handleScroll, true);
            window.removeEventListener('resize', handleScroll);
        };
    }, [isOpen, pv.metadata.uid, onOpenChange]);

    const toggleMenu = (e) => {
        e.stopPropagation();
        if (!isOpen) {
            const rect = buttonRef.current.getBoundingClientRect();
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
            id={`pv-menu-${pv.metadata.uid}`}
            className="fixed w-48 bg-[#2d2d2d] border border-[#3d3d3d] rounded-md shadow-lg z-50 py-1"
            style={{ top: position.top, left: position.left }}
            onClick={(e) => e.stopPropagation()}
        >
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onEditYaml(pv)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <PencilSquareIcon className="h-4 w-4" />
                Edit YAML
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onShowDependencies(pv)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <ShareIcon className="h-4 w-4" />
                Dependencies
            </button>
            <div className="h-px bg-[#3d3d3d] my-1" />
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onDelete(pv)); }}
                className="w-full text-left px-4 py-2 text-sm text-red-400 hover:bg-[#3d3d3d] flex items-center gap-2"
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
