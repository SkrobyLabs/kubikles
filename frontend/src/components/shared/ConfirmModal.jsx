import React from 'react';
import { createPortal } from 'react-dom';
import { useUI } from '../../context/UIContext';
import { ExclamationTriangleIcon } from '@heroicons/react/24/outline';

export default function ConfirmModal() {
    const { modal, closeModal } = useUI();

    if (!modal) return null;

    const { title, content, onConfirm, confirmText = 'Confirm', confirmStyle = 'danger' } = modal;

    const handleConfirm = async () => {
        if (onConfirm) {
            await onConfirm();
        }
    };

    const handleCancel = () => {
        closeModal();
    };

    const confirmButtonClass = confirmStyle === 'danger'
        ? 'bg-red-600 hover:bg-red-700 text-white'
        : 'bg-blue-600 hover:bg-blue-700 text-white';

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
            <div className="absolute inset-0 bg-black/50" onClick={handleCancel} />
            <div className="relative bg-surface-light border border-border rounded-lg shadow-xl max-w-md w-full mx-4">
                <div className="p-4">
                    <div className="flex items-start gap-3">
                        <div className="flex-shrink-0">
                            <ExclamationTriangleIcon className="h-6 w-6 text-yellow-500" />
                        </div>
                        <div className="flex-1">
                            <h3 className="text-lg font-medium text-white mb-2">{title}</h3>
                            <div className="text-sm text-gray-400">{content}</div>
                        </div>
                    </div>
                </div>
                <div className="flex justify-end gap-2 px-4 py-3 border-t border-border">
                    <button
                        onClick={handleCancel}
                        className="px-4 py-2 text-sm text-gray-300 hover:text-white hover:bg-surface-hover rounded transition-colors"
                    >
                        Cancel
                    </button>
                    {onConfirm && (
                        <button
                            onClick={handleConfirm}
                            className={`px-4 py-2 text-sm rounded transition-colors ${confirmButtonClass}`}
                        >
                            {confirmText}
                        </button>
                    )}
                </div>
            </div>
        </div>,
        document.body
    );
}
