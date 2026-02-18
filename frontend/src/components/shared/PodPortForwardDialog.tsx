import React, { useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { XMarkIcon, SignalIcon, ArrowPathIcon, ArrowTopRightOnSquareIcon } from '@heroicons/react/24/outline';
import { AddPortForwardConfig, UpdatePortForwardConfig, GetRandomAvailablePort, StartPortForward } from 'wailsjs/go/main/App';
import { BrowserOpenURL } from 'wailsjs/runtime/runtime';
import { useForm, podPortForwardSchema } from '~/lib/validation';

export default function PodPortForwardDialog({
    open,
    onOpenChange,
    pod,
    containerPort,
    currentContext,
    existingConfig = null  // Pass existing config for edit mode
}: { open: any; onOpenChange: any; pod: any; containerPort: any; currentContext: any; existingConfig?: any }) {
    const isEditing = !!existingConfig;

    const remotePort = containerPort?.containerPort || 0;
    const portName = containerPort?.name || '';

    const form = useForm({
        schema: podPortForwardSchema,
        initialValues: { localPort: 0, label: '', https: false, favorite: false, autoStart: false, keepAlive: false, startNow: true, openInBrowser: false },
        onSubmit: async (values) => {
            if (isEditing && existingConfig) {
                // Update existing config
                const updatedConfig = {
                    ...existingConfig,
                    localPort: values.localPort,
                    label: values.label,
                    favorite: values.favorite,
                    https: values.https,
                    autoStart: values.autoStart,
                    keepAlive: values.keepAlive
                };
                await UpdatePortForwardConfig(updatedConfig);
            } else {
                // Create new config
                const config = {
                    context: currentContext,
                    namespace: pod.metadata?.namespace,
                    resourceType: 'pod',
                    resourceName: pod.metadata?.name,
                    localPort: values.localPort,
                    remotePort: remotePort,
                    label: values.label,
                    favorite: values.favorite,
                    https: values.https,
                    autoStart: values.autoStart,
                    keepAlive: values.keepAlive
                };

                const result = await AddPortForwardConfig(config);

                // Start immediately if requested
                if (values.startNow && result?.id) {
                    try {
                        await StartPortForward(result.id);
                        // Open in browser if requested
                        if (values.openInBrowser) {
                            const protocol = values.https ? 'https' : 'http';
                            BrowserOpenURL(`${protocol}://localhost:${values.localPort}`);
                        }
                    } catch (startErr) {
                        console.error('Failed to start port forward:', startErr);
                    }
                }
            }

            onOpenChange(false);
        },
    });

    // Initialize form when dialog opens
    useEffect(() => {
        if (open && containerPort) {
            if (isEditing && existingConfig) {
                // Edit mode - populate from existing config
                form.reset({
                    label: existingConfig.label || '',
                    localPort: existingConfig.localPort || remotePort,
                    https: existingConfig.https || false,
                    favorite: existingConfig.favorite || false,
                    autoStart: existingConfig.autoStart || false,
                    keepAlive: existingConfig.keepAlive || false,
                    startNow: false,
                    openInBrowser: false
                });
            } else {
                // Create mode - generate defaults
                const defaultLabel = portName
                    ? `${pod.metadata?.name}:${portName}`
                    : `${pod.metadata?.name}:${remotePort}`;

                form.reset({
                    label: defaultLabel,
                    localPort: 0,
                    https: false,
                    favorite: false,
                    autoStart: false,
                    keepAlive: false,
                    startNow: true,
                    openInBrowser: false
                });

                // Get a random available local port (avoiding well-known ports)
                GetRandomAvailablePort()
                    .then((port: any) => form.setValue('localPort', port))
                    .catch((err: any) => {
                        console.error('Failed to get available port:', err);
                        form.setValue('localPort', remotePort);
                    });
            }
        }
    }, [open, containerPort, pod, remotePort, portName, isEditing, existingConfig]);

    const handleClose = useCallback(() => {
        onOpenChange(false);
    }, [onOpenChange]);

    const randomizePort = useCallback(async () => {
        try {
            const port = await GetRandomAvailablePort();
            form.setValue('localPort', port);
        } catch (err: any) {
            console.error('Failed to get random port:', err);
        }
    }, []);

    const handleOpenBrowser = useCallback(() => {
        const protocol = form.values.https ? 'https' : 'http';
        BrowserOpenURL(`${protocol}://localhost:${form.values.localPort}`);
    }, [form.values.https, form.values.localPort]);

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

                <form onSubmit={form.handleSubmit} className="space-y-4">
                    {/* Label */}
                    <div>
                        <label className="block text-sm font-medium text-gray-300 mb-1">
                            Label
                        </label>
                        <input
                            type="text"
                            {...form.getFieldProps('label') as any}
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
                                        title={`Open ${form.values.https ? 'https' : 'http'}://localhost:${form.values.localPort}`}
                                    >
                                        <ArrowTopRightOnSquareIcon className="w-3.5 h-3.5" />
                                    </button>
                                )}
                            </label>
                            <input
                                type="number"
                                value={form.values.localPort || ''}
                                onChange={(e: any) => form.setValue('localPort', parseInt(e.target.value) || 0)}
                                onBlur={() => form.setFieldTouched('localPort')}
                                min="1"
                                max="65535"
                                className="w-full px-3 py-2 bg-background border border-border rounded text-sm focus:outline-none focus:border-primary"
                            />
                            {form.touched.localPort && form.errors.localPort && (
                                <span className="text-red-400 text-xs mt-1 block">{form.errors.localPort}</span>
                            )}
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
                                        {...form.getFieldProps('startNow') as any}
                                        checked={form.values.startNow}
                                        className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                                    />
                                    <span className="text-sm text-gray-300">Start immediately</span>
                                </label>
                                <label className={`flex items-center gap-2 ${form.values.startNow ? 'cursor-pointer' : 'cursor-not-allowed opacity-50'}`}>
                                    <input
                                        type="checkbox"
                                        {...form.getFieldProps('openInBrowser') as any}
                                        checked={form.values.openInBrowser}
                                        disabled={!form.values.startNow}
                                        className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                                    />
                                    <span className="text-sm text-gray-300">Open in browser</span>
                                </label>
                            </>
                        )}
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                {...form.getFieldProps('autoStart') as any}
                                checked={form.values.autoStart}
                                className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                            />
                            <span className="text-sm text-gray-300">Auto-start on context switch</span>
                        </label>
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                {...form.getFieldProps('keepAlive') as any}
                                checked={form.values.keepAlive}
                                className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                            />
                            <span className="text-sm text-gray-300">Keep alive across contexts</span>
                        </label>
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                {...form.getFieldProps('https') as any}
                                checked={form.values.https}
                                className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                            />
                            <span className="text-sm text-gray-300">Use HTTPS for browser</span>
                        </label>
                        <label className="flex items-center gap-2 cursor-pointer">
                            <input
                                type="checkbox"
                                {...form.getFieldProps('favorite') as any}
                                checked={form.values.favorite}
                                className="w-4 h-4 rounded border-gray-600 bg-gray-800 text-primary focus:ring-primary"
                            />
                            <span className="text-sm text-gray-300">Add to favorites</span>
                        </label>
                    </div>

                    {/* Error */}
                    {form.submitError && (
                        <div className="text-sm text-red-400 bg-red-500/10 px-3 py-2 rounded">
                            {form.submitError}
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
                            disabled={form.isSubmitting || !form.isValid}
                            className="px-4 py-2 text-sm bg-primary hover:bg-primary/80 text-white rounded transition-colors disabled:opacity-50"
                        >
                            {form.isSubmitting ? (isEditing ? 'Saving...' : 'Creating...') : (isEditing ? 'Save' : 'Create Port Forward')}
                        </button>
                    </div>
                </form>
            </div>
        </div>,
        document.body
    );
}
