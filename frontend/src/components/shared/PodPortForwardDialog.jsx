import React, { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { XMarkIcon, SignalIcon, ArrowPathIcon, ArrowTopRightOnSquareIcon } from '@heroicons/react/24/outline';
import { AddPortForwardConfig, UpdatePortForwardConfig, GetRandomAvailablePort, StartPortForward } from '../../../wailsjs/go/main/App';
import { BrowserOpenURL } from '../../../wailsjs/runtime/runtime';

export default function PodPortForwardDialog({
    open,
    onOpenChange,
    pod,
    containerPort,
    currentContext,
    existingConfig = null  // Pass existing config for edit mode
}) {
    const isEditing = !!existingConfig;
    const [localPort, setLocalPort] = useState(0);
    const [label, setLabel] = useState('');
    const [https, setHttps] = useState(false);
    const [favorite, setFavorite] = useState(false);
    const [autoStart, setAutoStart] = useState(true);
    const [openInBrowser, setOpenInBrowser] = useState(false);
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);

    const remotePort = containerPort?.containerPort || 0;
    const portName = containerPort?.name || '';

    // Initialize form when dialog opens
    useEffect(() => {
        if (open && containerPort) {
            if (isEditing && existingConfig) {
                // Edit mode - populate from existing config
                setLabel(existingConfig.label || '');
                setLocalPort(existingConfig.localPort || remotePort);
                setHttps(existingConfig.https || false);
                setFavorite(existingConfig.favorite || false);
                setAutoStart(false);  // Don't auto-start when editing
                setOpenInBrowser(false);
            } else {
                // Create mode - generate defaults
                const defaultLabel = portName
                    ? `${pod.metadata?.name}:${portName}`
                    : `${pod.metadata?.name}:${remotePort}`;
                setLabel(defaultLabel);

                // Get a random available local port (avoiding well-known ports)
                GetRandomAvailablePort()
                    .then(port => setLocalPort(port))
                    .catch(err => {
                        console.error('Failed to get available port:', err);
                        setLocalPort(remotePort);
                    });

                setHttps(false);
                setFavorite(false);
                setAutoStart(true);
                setOpenInBrowser(false);
            }
            setError('');
        }
    }, [open, containerPort, pod, remotePort, portName, isEditing, existingConfig]);

    const handleSubmit = async (e) => {
        e.preventDefault();
        setError('');

        if (!localPort || localPort < 1 || localPort > 65535) {
            setError('Local port must be between 1 and 65535');
            return;
        }

        setSaving(true);
        try {
            if (isEditing && existingConfig) {
                // Update existing config
                const updatedConfig = {
                    ...existingConfig,
                    localPort: localPort,
                    label: label,
                    favorite: favorite,
                    https: https
                };
                await UpdatePortForwardConfig(updatedConfig);
            } else {
                // Create new config
                const config = {
                    context: currentContext,
                    namespace: pod.metadata?.namespace,
                    resourceType: 'pod',
                    resourceName: pod.metadata?.name,
                    localPort: localPort,
                    remotePort: remotePort,
                    label: label,
                    favorite: favorite,
                    https: https
                };

                const result = await AddPortForwardConfig(config);

                // Auto-start if requested
                if (autoStart && result?.id) {
                    try {
                        await StartPortForward(result.id);
                        // Open in browser if requested
                        if (openInBrowser) {
                            const protocol = https ? 'https' : 'http';
                            BrowserOpenURL(`${protocol}://localhost:${localPort}`);
                        }
                    } catch (startErr) {
                        console.error('Failed to auto-start port forward:', startErr);
                        // Don't fail the whole operation, just log
                    }
                }
            }

            onOpenChange(false);
        } catch (err) {
            setError(err.message || (isEditing ? 'Failed to update port forward' : 'Failed to create port forward'));
        } finally {
            setSaving(false);
        }
    };

    const handleClose = useCallback(() => {
        onOpenChange(false);
    }, [onOpenChange]);

    const randomizePort = useCallback(async () => {
        try {
            const port = await GetRandomAvailablePort();
            setLocalPort(port);
        } catch (err) {
            console.error('Failed to get random port:', err);
        }
    }, []);

    const handleOpenBrowser = useCallback(() => {
        const protocol = https ? 'https' : 'http';
        BrowserOpenURL(`${protocol}://localhost:${localPort}`);
    }, [https, localPort]);

    if (!open) return null;

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
            <div className="absolute inset-0 bg-black/50" onClick={handleClose} />
            <div className="relative bg-surface border border-border rounded-lg shadow-xl w-full max-w-sm mx-4 p-6">
                <div className="flex items-center justify-between mb-4">
                    <div className="flex items-center gap-2">
                        <SignalIcon className="w-5 h-5 text-primary" />
                        <h2 className="text-lg font-semibold">{isEditing ? 'Edit Port Forward' : 'Create Port Forward'}</h2>
                    </div>
                    <button
                        onClick={handleClose}
                        className="p-1 text-gray-400 hover:text-white rounded hover:bg-white/10"
                    >
                        <XMarkIcon className="w-5 h-5" />
                    </button>
                </div>

                {/* Pod info */}
                <div className="mb-4 p-3 bg-background rounded-lg text-sm">
                    <div className="text-gray-400 mb-1">Pod</div>
                    <div className="text-white font-medium">{pod.metadata?.namespace}/{pod.metadata?.name}</div>
                </div>

                <form onSubmit={handleSubmit} className="space-y-4">
                    {/* Label */}
                    <div>
                        <label className="block text-sm font-medium text-gray-300 mb-1">
                            Label
                        </label>
                        <input
                            type="text"
                            value={label}
                            onChange={(e) => setLabel(e.target.value)}
                            className="w-full px-3 py-2 bg-background border border-border rounded text-sm focus:outline-none focus:border-primary"
                        />
                    </div>

                    {/* Ports */}
                    <div className="grid grid-cols-2 gap-4">
                        <div>
                            <label className="flex items-center gap-1 text-sm font-medium text-gray-300 mb-1">
                                Local Port
                                <button
                                    type="button"
                                    onClick={randomizePort}
                                    className="p-0.5 text-gray-500 hover:text-primary transition-colors"
                                    title="Randomize port"
                                >
                                    <ArrowPathIcon className="w-3.5 h-3.5" />
                                </button>
                                {isEditing && (
                                    <button
                                        type="button"
                                        onClick={handleOpenBrowser}
                                        className="p-0.5 text-gray-500 hover:text-primary transition-colors"
                                        title={`Open ${https ? 'https' : 'http'}://localhost:${localPort}`}
                                    >
                                        <ArrowTopRightOnSquareIcon className="w-3.5 h-3.5" />
                                    </button>
                                )}
                            </label>
                            <input
                                type="number"
                                value={localPort || ''}
                                onChange={(e) => setLocalPort(parseInt(e.target.value) || 0)}
                                min="1"
                                max="65535"
                                className="w-full px-3 py-2 bg-background border border-border rounded text-sm focus:outline-none focus:border-primary"
                            />
                        </div>
                        <div>
                            <label className="block text-sm font-medium text-gray-300 mb-1">
                                Remote Port
                            </label>
                            <input
                                type="number"
                                value={remotePort}
                                disabled
                                className="w-full px-3 py-2 bg-background border border-border rounded text-sm text-gray-500 cursor-not-allowed"
                            />
                        </div>
                    </div>

                    {/* Options */}
                    <div className="space-y-2">
                        {!isEditing && (
                            <>
                                <label className="flex items-center gap-2 cursor-pointer">
                                    <input
                                        type="checkbox"
                                        checked={autoStart}
                                        onChange={(e) => setAutoStart(e.target.checked)}
                                        className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                                    />
                                    <span className="text-sm text-gray-300">Start immediately</span>
                                </label>
                                <label className={`flex items-center gap-2 ${autoStart ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'}`}>
                                    <input
                                        type="checkbox"
                                        checked={openInBrowser}
                                        onChange={(e) => setOpenInBrowser(e.target.checked)}
                                        disabled={!autoStart}
                                        className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                                    />
                                    <span className="text-sm text-gray-300">Open in browser</span>
                                </label>
                            </>
                        )}
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                checked={https}
                                onChange={(e) => setHttps(e.target.checked)}
                                className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                            />
                            <span className="text-sm text-gray-300">Use HTTPS for browser</span>
                        </label>
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                checked={favorite}
                                onChange={(e) => setFavorite(e.target.checked)}
                                className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                            />
                            <span className="text-sm text-gray-300">Add to favorites</span>
                        </label>
                    </div>

                    {/* Error */}
                    {error && (
                        <div className="text-sm text-red-400 bg-red-500/10 px-3 py-2 rounded">
                            {error}
                        </div>
                    )}

                    {/* Actions */}
                    <div className="flex justify-end gap-3 pt-2">
                        <button
                            type="button"
                            onClick={handleClose}
                            className="px-4 py-2 text-sm text-gray-300 hover:text-white transition-colors"
                        >
                            Cancel
                        </button>
                        <button
                            type="submit"
                            disabled={saving}
                            className="px-4 py-2 text-sm bg-primary hover:bg-primary/80 text-white rounded transition-colors disabled:opacity-50"
                        >
                            {saving ? (isEditing ? 'Saving...' : 'Creating...') : (isEditing ? 'Save' : 'Create Port Forward')}
                        </button>
                    </div>
                </form>
            </div>
        </div>,
        document.body
    );
}
