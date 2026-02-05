import React, { useState, useRef, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { Cog6ToothIcon } from '@heroicons/react/24/outline';

/**
 * Sound definitions for notifications.
 * Each sound is defined by its note sequence.
 */
export const NOTIFICATION_SOUNDS = {
    chime: {
        label: 'Chime',
        notes: [
            { freq: 523, start: 0 },
            { freq: 659, start: 0.15 },
            { freq: 784, start: 0.30 }
        ]
    },
    alert: {
        label: 'Alert',
        notes: [
            { freq: 880, start: 0 },
            { freq: 660, start: 0.18 }
        ]
    },
    ding: {
        label: 'Ding',
        notes: [
            { freq: 1047, start: 0 }
        ]
    },
    double: {
        label: 'Double Beep',
        notes: [
            { freq: 800, start: 0 },
            { freq: 800, start: 0.15 }
        ]
    },
    ascending: {
        label: 'Ascending',
        notes: [
            { freq: 440, start: 0 },
            { freq: 554, start: 0.12 },
            { freq: 659, start: 0.24 },
            { freq: 880, start: 0.36 }
        ]
    },
    soft: {
        label: 'Soft',
        notes: [
            { freq: 392, start: 0 },
            { freq: 523, start: 0.2 }
        ]
    },
    none: {
        label: 'None (Silent)',
        notes: []
    }
};

/**
 * Floating configuration menu for notification settings.
 * Opens on right-click of the notification bell.
 */
export default function NotificationSettingsMenu({
    isOpen,
    onClose,
    position,
    throttleSeconds,
    onThrottleChange,
    selectedSound,
    onSoundChange,
    onPreviewSound,
}) {
    const menuRef = useRef(null);

    // Close on outside click (with delay to avoid catching the opening click)
    useEffect(() => {
        if (!isOpen) return;
        const handleClick = (e) => {
            if (menuRef.current && !menuRef.current.contains(e.target)) {
                onClose();
            }
        };
        // Delay adding listener to avoid catching the right-click that opened the menu
        const timeoutId = setTimeout(() => {
            document.addEventListener('mousedown', handleClick);
        }, 10);
        return () => {
            clearTimeout(timeoutId);
            document.removeEventListener('mousedown', handleClick);
        };
    }, [isOpen, onClose]);

    // Close on Escape
    useEffect(() => {
        if (!isOpen) return;
        const handleKeyDown = (e) => {
            if (e.key === 'Escape') onClose();
        };
        document.addEventListener('keydown', handleKeyDown);
        return () => document.removeEventListener('keydown', handleKeyDown);
    }, [isOpen, onClose]);

    if (!isOpen) return null;

    return createPortal(
        <div
            ref={menuRef}
            className="fixed bg-surface border border-border rounded-lg shadow-xl z-[200] w-64 py-2"
            style={{
                top: position.y,
                left: position.x,
            }}
        >
            {/* Header */}
            <div className="px-3 pb-2 mb-2 border-b border-border flex items-center gap-2">
                <Cog6ToothIcon className="w-4 h-4 text-gray-400" />
                <span className="text-sm font-medium text-gray-300">Notification Settings</span>
            </div>

            {/* Throttle Setting */}
            <div className="px-3 py-2">
                <label className="block text-xs text-gray-400 mb-1.5">
                    Minimum interval between alerts
                </label>
                <div className="flex items-center gap-2">
                    <input
                        type="range"
                        min="0"
                        max="60"
                        step="1"
                        value={throttleSeconds}
                        onChange={(e) => onThrottleChange(Number(e.target.value))}
                        className="flex-1 h-1.5 bg-gray-700 rounded-lg appearance-none cursor-pointer accent-primary"
                    />
                    <span className="text-sm text-gray-300 w-12 text-right tabular-nums">
                        {throttleSeconds === 0 ? 'None' : `${throttleSeconds}s`}
                    </span>
                </div>
            </div>

            {/* Sound Selection */}
            <div className="px-3 py-2">
                <label className="block text-xs text-gray-400 mb-1.5">
                    Alert sound
                </label>
                <div className="flex items-center gap-2">
                    <select
                        value={selectedSound}
                        onChange={(e) => onSoundChange(e.target.value)}
                        className="flex-1 px-2 py-1.5 bg-background border border-border rounded text-sm text-gray-300 focus:outline-none focus:border-primary cursor-pointer"
                    >
                        {Object.entries(NOTIFICATION_SOUNDS).map(([key, { label }]) => (
                            <option key={key} value={key}>{label}</option>
                        ))}
                    </select>
                    <button
                        onClick={() => onPreviewSound(selectedSound)}
                        disabled={selectedSound === 'none'}
                        className="px-2 py-1.5 text-xs bg-white/5 hover:bg-white/10 text-gray-300 rounded border border-border transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                        Test
                    </button>
                </div>
            </div>

            {/* Info */}
            <div className="px-3 pt-2 mt-1 border-t border-border">
                <p className="text-[11px] text-gray-500 leading-relaxed">
                    Notifications trigger on pod restarts and redeploys for visible pods.
                </p>
            </div>
        </div>,
        document.body
    );
}
