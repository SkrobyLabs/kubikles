import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { PencilSquareIcon, ArrowPathIcon, TrashIcon, EllipsisVerticalIcon, DocumentTextIcon, ShareIcon } from '@heroicons/react/24/outline';
import ComparisonMenuItems from '~/components/shared/ComparisonMenuItems';
import type { K8sDeployment } from '~/types/k8s';

interface MenuPosition {
    top: number;
    left: number;
}

interface DeploymentActionsMenuProps {
    deployment: K8sDeployment;
    isOpen: boolean;
    menuPosition: MenuPosition;
    onOpenChange: (isOpen: boolean, buttonEl?: HTMLElement | null) => void;
    onEditYaml: (deployment: K8sDeployment) => void;
    onShowDependencies: (deployment: K8sDeployment) => void;
    onRestart: (deployment: K8sDeployment) => void;
    onDelete: (deployment: K8sDeployment) => void;
    onViewLogs: (deployment: K8sDeployment) => void;
}

export default function DeploymentActionsMenu({
    deployment,
    isOpen,
    menuPosition,
    onOpenChange,
    onEditYaml,
    onShowDependencies,
    onRestart,
    onDelete,
    onViewLogs
}: DeploymentActionsMenuProps) {
    const buttonRef = useRef<HTMLButtonElement>(null);
    const menuRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        if (!isOpen) return;

        const handleClickOutside = (event: MouseEvent) => {
            if (buttonRef.current && !buttonRef.current.contains(event.target as Node) &&
                menuRef.current && !menuRef.current.contains(event.target as Node)) {
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

    const toggleMenu = (e: React.MouseEvent) => {
        e.stopPropagation();
        onOpenChange(!isOpen, buttonRef.current);
    };

    const handleAction = (action: () => void) => {
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
                onClick={(e) => { e.stopPropagation(); handleAction(() => onViewLogs(deployment)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <DocumentTextIcon className="h-4 w-4" />
                View Logs
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onEditYaml(deployment)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <PencilSquareIcon className="h-4 w-4" />
                Edit
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onShowDependencies(deployment)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <ShareIcon className="h-4 w-4" />
                Dependencies
            </button>
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onRestart(deployment)); }}
                className="w-full text-left px-4 py-2 text-sm text-gray-300 hover:bg-surface-hover flex items-center gap-2"
            >
                <ArrowPathIcon className="h-4 w-4" />
                Restart
            </button>
            <div className="h-px bg-surface-hover my-1" />
            <ComparisonMenuItems
                kind="deployment"
                namespace={deployment.metadata?.namespace}
                name={deployment.metadata?.name}
                onAction={() => onOpenChange(false)}
            />
            <div className="h-px bg-surface-hover my-1" />
            <button
                onClick={(e) => { e.stopPropagation(); handleAction(() => onDelete(deployment)); }}
                className="w-full text-left px-4 py-2 text-sm text-red-400 hover:bg-surface-hover flex items-center gap-2"
            >
                <TrashIcon className="h-4 w-4" />
                Delete
            </button>
        </div >
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
