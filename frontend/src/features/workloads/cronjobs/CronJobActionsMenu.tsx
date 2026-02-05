import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { PencilSquareIcon, TrashIcon, EllipsisVerticalIcon, DocumentTextIcon, PlayIcon, PauseIcon, ShareIcon } from '@heroicons/react/24/outline';

export default function CronJobActionsMenu({ cronJob, isOpen, menuPosition, onOpenChange, onViewLogs, onEditYaml, onShowDependencies, onRunNow, onSuspend, onDelete }) {
    const buttonRef = useRef(null);
    const menuRef = useRef(null);
    const isSuspended = cronJob.spec?.suspend || false;

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

    const menu = (
        <div
            ref={menuRef}
            className="w-48 bg-surface-light border border-border rounded-md shadow-lg py-1"
            style={{ position: 'fixed', top: `${menuPosition.top}px`, left: `${menuPosition.left}px`, zIndex: 99999 }}
            onClick={(e) => e.stopPropagation()}
        >
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onViewLogs(cronJob)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <DocumentTextIcon className="h-4 w-4" />
                View Logs
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onEditYaml(cronJob)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <PencilSquareIcon className="h-4 w-4" />
                Edit
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onShowDependencies(cronJob)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <ShareIcon className="h-4 w-4" />
                Dependencies
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onRunNow(cronJob)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <PlayIcon className="h-4 w-4" />
                Run Now
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onSuspend(cronJob)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                {isSuspended ? <PlayIcon className="h-4 w-4" /> : <PauseIcon className="h-4 w-4" />}
                {isSuspended ? 'Resume' : 'Suspend'}
            </button>
            <div className="h-px bg-surface-hover my-1" />
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onDelete(cronJob)); }}
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
