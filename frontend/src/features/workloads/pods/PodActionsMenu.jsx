import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { PencilSquareIcon, DocumentTextIcon, CommandLineIcon, TrashIcon, EllipsisVerticalIcon, ShareIcon } from '@heroicons/react/24/outline';

export default function PodActionsMenu({ pod, isOpen, menuPosition, onOpenChange, onLogs, onEditYaml, onShowDependencies, onDelete, onForceDelete, onShell }) {
    const buttonRef = useRef(null);
    const menuRef = useRef(null);

    useEffect(() => {
        if (!isOpen) return;

        const handleClickOutside = (event) => {
            if (buttonRef.current && !buttonRef.current.contains(event.target) &&
                menuRef.current && !menuRef.current.contains(event.target)) {
                onOpenChange(false);
            }
        };

        const handleScroll = () => {
            onOpenChange(false);
        };

        document.addEventListener('mousedown', handleClickOutside);
        window.addEventListener('scroll', handleScroll, true);

        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
            window.removeEventListener('scroll', handleScroll, true);
        };
    }, [isOpen, onOpenChange]);

    const toggleMenu = (e) => {
        e.stopPropagation();
        onOpenChange(!isOpen, buttonRef.current);
    };

    const handleAction = (action) => {
        onOpenChange(false);
        action();
    };

    const isTerminating = pod.metadata.deletionTimestamp;

    const menu = (
        <div
            ref={menuRef}
            className="w-48 bg-[#2d2d2d] border border-[#3d3d3d] rounded-md shadow-lg py-1"
            style={{ position: 'fixed', top: `${menuPosition.top}px`, left: `${menuPosition.left}px`, zIndex: 99999 }}
            onClick={(e) => e.stopPropagation()}
        >
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onLogs(pod)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <DocumentTextIcon className="h-4 w-4" />
                View Logs
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onEditYaml(pod)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <PencilSquareIcon className="h-4 w-4" />
                Edit
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onShowDependencies(pod)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <ShareIcon className="h-4 w-4" />
                Dependencies
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onShell(pod)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <CommandLineIcon className="h-4 w-4" />
                Shell
            </button>
            <div className="h-px bg-[#3d3d3d] my-1" />
            <button
                onClick={(e) => {
                    e.stopPropagation();
                    if (isTerminating) {
                        handleAction(() => onForceDelete(pod));
                    } else {
                        handleAction(() => onDelete(pod));
                    }
                }}
                className={`w-full text-left px-4 py-2 text-sm hover:bg-[#3d3d3d] flex items-center gap-2 transition-colors ${isTerminating ? 'text-red-500 font-semibold' : 'text-red-400'}`}
            >
                <TrashIcon className="h-4 w-4" />
                {isTerminating ? 'Force Delete' : 'Delete'}
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
