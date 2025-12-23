import React, { useState, useCallback } from 'react';
import { createPortal } from 'react-dom';
import {
    XMarkIcon,
    CheckCircleIcon,
    ExclamationTriangleIcon,
    XCircleIcon,
    InformationCircleIcon,
    ClipboardIcon,
    ClipboardDocumentCheckIcon
} from '@heroicons/react/24/outline';
import { useNotification } from '../../context/NotificationContext';

const typeStyles = {
    success: {
        icon: CheckCircleIcon,
        iconClass: 'text-green-400',
        borderClass: 'border-green-500/30',
        bgClass: 'bg-green-500/10'
    },
    error: {
        icon: XCircleIcon,
        iconClass: 'text-red-400',
        borderClass: 'border-red-500/30',
        bgClass: 'bg-red-500/10'
    },
    warning: {
        icon: ExclamationTriangleIcon,
        iconClass: 'text-yellow-400',
        borderClass: 'border-yellow-500/30',
        bgClass: 'bg-yellow-500/10'
    },
    info: {
        icon: InformationCircleIcon,
        iconClass: 'text-blue-400',
        borderClass: 'border-blue-500/30',
        bgClass: 'bg-blue-500/10'
    }
};

function Toast({ notification, onClose }) {
    const [copied, setCopied] = useState(false);
    const { type = 'info', title, message } = notification;
    const styles = typeStyles[type] || typeStyles.info;
    const Icon = styles.icon;

    const handleCopy = useCallback(async () => {
        const text = `${title}${message ? '\n' + message : ''}`;
        try {
            await navigator.clipboard.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    }, [title, message]);

    return (
        <div
            className={`
                max-w-md w-full bg-surface border ${styles.borderClass} rounded-lg shadow-lg
                transform transition-all duration-300 ease-out
                animate-slide-in
            `}
        >
            <div className={`flex items-start gap-3 p-4 ${styles.bgClass} rounded-lg`}>
                <Icon className={`h-5 w-5 ${styles.iconClass} shrink-0 mt-0.5`} />
                <div className="flex-1 min-w-0">
                    {title && (
                        <div className={`font-medium text-sm ${styles.iconClass}`}>
                            {title}
                        </div>
                    )}
                    {message && (
                        <div className="text-sm text-gray-300 mt-1 break-words whitespace-pre-wrap">
                            {message}
                        </div>
                    )}
                </div>
                <div className="flex items-center gap-1 shrink-0">
                    <button
                        onClick={handleCopy}
                        className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                        title="Copy to clipboard"
                    >
                        {copied ? (
                            <ClipboardDocumentCheckIcon className="h-4 w-4 text-green-400" />
                        ) : (
                            <ClipboardIcon className="h-4 w-4" />
                        )}
                    </button>
                    <button
                        onClick={onClose}
                        className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors"
                        title="Dismiss"
                    >
                        <XMarkIcon className="h-4 w-4" />
                    </button>
                </div>
            </div>
        </div>
    );
}

export default function ToastContainer() {
    const { notifications, removeNotification } = useNotification();

    if (notifications.length === 0) return null;

    return createPortal(
        <div className="fixed bottom-4 right-4 z-[99999] flex flex-col gap-2">
            {notifications.map((notification) => (
                <Toast
                    key={notification.id}
                    notification={notification}
                    onClose={() => removeNotification(notification.id)}
                />
            ))}
        </div>,
        document.body
    );
}
