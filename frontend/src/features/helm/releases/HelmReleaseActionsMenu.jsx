import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import {
    EllipsisVerticalIcon,
    DocumentTextIcon,
    TrashIcon,
    ArrowUturnLeftIcon,
    ClockIcon,
    InformationCircleIcon
} from '@heroicons/react/24/outline';

export default function HelmReleaseActionsMenu({
    release,
    isOpen,
    menuPosition,
    onOpenChange,
    onViewDetails,
    onViewValues,
    onViewHistory,
    onRollback,
    onUninstall
}) {
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

    const menu = (
        <div
            ref={menuRef}
            className="w-48 bg-[#2d2d2d] border border-[#3d3d3d] rounded-md shadow-lg py-1"
            style={{ position: 'fixed', top: `${menuPosition.top}px`, left: `${menuPosition.left}px`, zIndex: 99999 }}
            onClick={(e) => e.stopPropagation()}
        >
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onViewDetails(release)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <InformationCircleIcon className="h-4 w-4" />
                Details
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onViewValues(release)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <DocumentTextIcon className="h-4 w-4" />
                View Values
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onViewHistory(release)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <ClockIcon className="h-4 w-4" />
                View History
            </button>
            {release.revision > 1 && (
                <button
                    onClick={(e) => { e.stopPropagation(); handleAction(() => onRollback(release)); }}
                    className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-[#3d3d3d] flex items-center gap-2"
                >
                    <ArrowUturnLeftIcon className="h-4 w-4" />
                    Rollback
                </button>
            )}
            <div className="h-px bg-[#3d3d3d] my-1" />
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onUninstall(release)); }}
                className="w-full text-left px-4 py-2 text-sm text-red-400 hover:bg-[#3d3d3d] flex items-center gap-2"
            >
                <TrashIcon className="h-4 w-4" />
                Uninstall
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
