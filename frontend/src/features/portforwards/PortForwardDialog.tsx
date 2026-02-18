import React, { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { XMarkIcon, ArrowPathIcon } from '@heroicons/react/24/outline';
import { GetRandomAvailablePort, GetPodPorts, GetServicePorts } from 'wailsjs/go/main/App';
import { useForm, portForwardConfigSchema } from '~/lib/validation';

export default function PortForwardDialog({
    open,
    onOpenChange,
    config,
    onSave,
    contexts,
    currentContext
}: any) {
    const [availablePorts, setAvailablePorts] = useState<any[]>([]);
    const [loadingPorts, setLoadingPorts] = useState(false);

    const isEditing = !!config;

    const form = useForm({
        schema: portForwardConfigSchema,
        initialValues: {
            id: '',
            context: currentContext || '',
            namespace: '',
            resourceType: 'pod' as const,
            resourceName: '',
            localPort: 0,
            remotePort: 0,
            label: '',
            favorite: false,
            https: false,
            autoStart: false,
            keepAlive: false,
            startNow: true,
            openInBrowser: false
        },
        onSubmit: async (values) => {
            await onSave(values);
        },
    });

    // Initialize form when dialog opens
    useEffect(() => {
        if (open) {
            if (config) {
                // Editing existing config
                form.reset({
                    id: config.id || '',
                    context: config.context || currentContext || '',
                    namespace: config.namespace || '',
                    resourceType: config.resourceType || 'pod',
                    resourceName: config.resourceName || '',
                    localPort: config.localPort || 0,
                    remotePort: config.remotePort || 0,
                    label: config.label || '',
                    favorite: config.favorite || false,
                    https: config.https || false,
                    autoStart: config.autoStart || false,
                    keepAlive: config.keepAlive || false,
                    startNow: false,
                    openInBrowser: false
                });
                setAvailablePorts([]);
            } else {
                // New config
                form.reset({
                    id: '',
                    context: currentContext || '',
                    namespace: '',
                    resourceType: 'pod',
                    resourceName: '',
                    localPort: 0,
                    remotePort: 0,
                    label: '',
                    favorite: false,
                    https: false,
                    autoStart: false,
                    keepAlive: false,
                    startNow: true,
                    openInBrowser: false
                });
                // Get a random available port
                GetRandomAvailablePort()
                    .then((port: any) => form.setValue('localPort', port))
                    .catch(console.error);
                setAvailablePorts([]);
            }
        }
    }, [open, config, currentContext]);

    // Fetch available ports when resource changes
    const fetchResourcePorts = useCallback(async () => {
        if (!form.values.namespace || !form.values.resourceName) {
            setAvailablePorts([]);
            return;
        }

        setLoadingPorts(true);
        try {
            let ports;
            if (form.values.resourceType === 'pod') {
                ports = await GetPodPorts(form.values.namespace, form.values.resourceName);
            } else {
                ports = await GetServicePorts(form.values.namespace, form.values.resourceName);
            }
            setAvailablePorts(ports || []);
            // Auto-select first port if none selected
            if (ports && ports.length > 0 && !form.values.remotePort) {
                form.setValue('remotePort', ports[0]);
            }
        } catch (err: any) {
            console.error('Failed to fetch ports:', err);
            setAvailablePorts([]);
        } finally {
            setLoadingPorts(false);
        }
    }, [form.values.namespace, form.values.resourceName, form.values.resourceType]);

    useEffect(() => {
        if (open && form.values.resourceName) {
            fetchResourcePorts();
        }
    }, [open, form.values.resourceName, fetchResourcePorts]);

    const handleClose = () => {
        onOpenChange(false);
    };

    const randomizePort = useCallback(async () => {
        try {
            const port = await GetRandomAvailablePort();
            form.setValue('localPort', port);
        } catch (err: any) {
            console.error('Failed to get random port:', err);
        }
    }, []);

    if (!open) return null;

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center">
            <div className="absolute inset-0 bg-black/50" onClick={handleClose} />
            <div className="relative bg-surface border border-border rounded-lg shadow-xl w-full max-w-md mx-4 p-6">
                <div className="flex items-center justify-between mb-6">
                    <h2 className="text-lg font-semibold">
                        {isEditing ? 'Edit Port Forward' : 'Add Port Forward'}
                    </h2>
                    <button
                        onClick={handleClose}
                        className="p-1 text-gray-400 hover:text-white rounded hover:bg-white/10"
                    >
                        <XMarkIcon className="w-5 h-5" />
                    </button>
                </div>

                <form onSubmit={form.handleSubmit} className="space-y-4">
                    {isEditing ? (
                        /* Edit mode - show resource info as read-only */
                        <>
                            {/* Resource Info */}
                            <div className="bg-surface-light rounded-lg p-3 space-y-2">
                                <div className="flex justify-between text-sm">
                                    <span className="text-gray-500">Resource</span>
                                    <span className="text-gray-200 capitalize">{form.values.resourceType}: {form.values.resourceName}</span>
                                </div>
                                <div className="flex justify-between text-sm">
                                    <span className="text-gray-500">Namespace</span>
                                    <span className="text-gray-200">{form.values.namespace}</span>
                                </div>
                                <div className="flex justify-between text-sm">
                                    <span className="text-gray-500">Remote Port</span>
                                    <span className="text-gray-200 font-mono">{form.values.remotePort}</span>
                                </div>
                            </div>

                            {/* Label */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    Label (optional)
                                </label>
                                <input
                                    type="text"
                                    {...form.getFieldProps('label')}
                                    placeholder="My Port Forward"
                                    className="w-full"
                                />
                            </div>

                            {/* Local Port */}
                            <div>
                                <label className="flex items-center gap-1 font-medium text-gray-300 mb-1">
                                    Local Port
                                    <button
                                        type="button"
                                        onClick={randomizePort}
                                        className="p-0.5 text-gray-500 hover:text-primary transition-colors"
                                        title="Randomize port"
                                    >
                                        <ArrowPathIcon className="w-3.5 h-3.5" />
                                    </button>
                                </label>
                                <input
                                    type="number"
                                    value={form.values.localPort || ''}
                                    onChange={(e: any) => form.setValue('localPort', parseInt(e.target.value) || 0)}
                                    onBlur={() => form.setFieldTouched('localPort')}
                                    placeholder="8080"
                                    min="1"
                                    max="65535"
                                    className="w-full"
                                />
                                {form.touched.localPort && form.errors.localPort && (
                                    <span className="text-red-400 text-xs mt-1 block">{form.errors.localPort}</span>
                                )}
                            </div>
                        </>
                    ) : (
                        /* Create mode - full form */
                        <>
                            {/* Label */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    Label (optional)
                                </label>
                                <input
                                    type="text"
                                    {...form.getFieldProps('label')}
                                    placeholder="My Port Forward"
                                    className="w-full"
                                />
                            </div>

                            {/* Context */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    Context
                                </label>
                                <select
                                    value={form.values.context}
                                    onChange={(e: any) => form.setValue('context', e.target.value)}
                                    onBlur={() => form.setFieldTouched('context')}
                                    className="w-full"
                                >
                                    <option value="">Select context...</option>
                                    {contexts.map((ctx: any) => (
                                        <option key={ctx} value={ctx}>{ctx}</option>
                                    ))}
                                </select>
                                {form.touched.context && form.errors.context && (
                                    <span className="text-red-400 text-xs mt-1 block">{form.errors.context}</span>
                                )}
                            </div>

                            {/* Namespace */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    Namespace
                                </label>
                                <input
                                    type="text"
                                    {...form.getFieldProps('namespace')}
                                    placeholder="default"
                                    className="w-full"
                                />
                                {form.touched.namespace && form.errors.namespace && (
                                    <span className="text-red-400 text-xs mt-1 block">{form.errors.namespace}</span>
                                )}
                            </div>

                            {/* Resource Type */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    Resource Type
                                </label>
                                <div className="flex gap-4">
                                    <label className="flex items-center gap-2">
                                        <input
                                            type="radio"
                                            name="resourceType"
                                            value="pod"
                                            checked={form.values.resourceType === 'pod'}
                                            onChange={(e: any) => form.setValue('resourceType', e.target.value as 'pod' | 'service')}
                                        />
                                        <span>Pod</span>
                                    </label>
                                    <label className="flex items-center gap-2">
                                        <input
                                            type="radio"
                                            name="resourceType"
                                            value="service"
                                            checked={form.values.resourceType === 'service'}
                                            onChange={(e: any) => form.setValue('resourceType', e.target.value as 'pod' | 'service')}
                                        />
                                        <span>Service</span>
                                    </label>
                                </div>
                            </div>

                            {/* Resource Name */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    {form.values.resourceType === 'pod' ? 'Pod Name' : 'Service Name'}
                                </label>
                                <input
                                    type="text"
                                    {...form.getFieldProps('resourceName')}
                                    placeholder={form.values.resourceType === 'pod' ? 'my-pod-abc123' : 'my-service'}
                                    className="w-full"
                                />
                                {form.touched.resourceName && form.errors.resourceName && (
                                    <span className="text-red-400 text-xs mt-1 block">{form.errors.resourceName}</span>
                                )}
                            </div>
                        </>
                    )}

                    {/* Ports - only show in create mode */}
                    {!isEditing && (
                    <div className="grid grid-cols-2 gap-4">
                        <div>
                            <label className="flex items-center gap-1 font-medium text-gray-300 mb-1">
                                Local Port
                                <button
                                    type="button"
                                    onClick={randomizePort}
                                    className="p-0.5 text-gray-500 hover:text-primary transition-colors"
                                    title="Randomize port"
                                >
                                    <ArrowPathIcon className="w-3.5 h-3.5" />
                                </button>
                            </label>
                            <input
                                type="number"
                                value={form.values.localPort || ''}
                                onChange={(e: any) => form.setValue('localPort', parseInt(e.target.value) || 0)}
                                onBlur={() => form.setFieldTouched('localPort')}
                                placeholder="8080"
                                min="1"
                                max="65535"
                                className="w-full"
                            />
                            {form.touched.localPort && form.errors.localPort && (
                                <span className="text-red-400 text-xs mt-1 block">{form.errors.localPort}</span>
                            )}
                        </div>
                        <div>
                            <label className="block font-medium text-gray-300 mb-1">
                                Remote Port
                                {loadingPorts && <span className="ml-2 text-xs text-gray-500">Loading...</span>}
                            </label>
                            {availablePorts.length > 0 ? (
                                <select
                                    value={form.values.remotePort || ''}
                                    onChange={(e: any) => form.setValue('remotePort', parseInt(e.target.value) || 0)}
                                    onBlur={() => form.setFieldTouched('remotePort')}
                                    className="w-full"
                                >
                                    <option value="">Select port...</option>
                                    {availablePorts.map((port: any) => (
                                        <option key={port} value={port}>{port}</option>
                                    ))}
                                </select>
                            ) : (
                                <input
                                    type="number"
                                    value={form.values.remotePort || ''}
                                    onChange={(e: any) => form.setValue('remotePort', parseInt(e.target.value) || 0)}
                                    onBlur={() => form.setFieldTouched('remotePort')}
                                    placeholder="80"
                                    min="1"
                                    max="65535"
                                    className="w-full"
                                />
                            )}
                            {form.touched.remotePort && form.errors.remotePort && (
                                <span className="text-red-400 text-xs mt-1 block">{form.errors.remotePort}</span>
                            )}
                        </div>
                    </div>
                    )}

                    {/* Options */}
                    <div className="space-y-2">
                        {!isEditing && (
                            <>
                                <label className="flex items-center gap-2">
                                    <input
                                        type="checkbox"
                                        {...form.getFieldProps('startNow')}
                                        checked={form.values.startNow}
                                    />
                                    <span className="text-gray-300">Start immediately</span>
                                </label>
                                <label className={`flex items-center gap-2 ${!form.values.startNow && 'opacity-50'}`}>
                                    <input
                                        type="checkbox"
                                        {...form.getFieldProps('openInBrowser')}
                                        checked={form.values.openInBrowser}
                                        disabled={!form.values.startNow}
                                    />
                                    <span className="text-gray-300">Open in browser</span>
                                </label>
                            </>
                        )}
                        <label className="flex items-center gap-2">
                            <input
                                type="checkbox"
                                {...form.getFieldProps('autoStart')}
                                checked={form.values.autoStart}
                            />
                            <span className="text-gray-300">Auto-start on context switch</span>
                        </label>
                        <label className="flex items-center gap-2">
                            <input
                                type="checkbox"
                                {...form.getFieldProps('keepAlive')}
                                checked={form.values.keepAlive}
                            />
                            <span className="text-gray-300">Keep alive across contexts</span>
                        </label>
                        <div className="flex gap-6">
                            <label className="flex items-center gap-2">
                                <input
                                    type="checkbox"
                                    {...form.getFieldProps('https')}
                                    checked={form.values.https}
                                />
                                <span className="text-gray-300">Use HTTPS</span>
                            </label>
                            <label className="flex items-center gap-2">
                                <input
                                    type="checkbox"
                                    {...form.getFieldProps('favorite')}
                                    checked={form.values.favorite}
                                />
                                <span className="text-gray-300">Favorite</span>
                            </label>
                        </div>
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
                            {form.isSubmitting ? 'Saving...' : isEditing ? 'Save Changes' : 'Add Port Forward'}
                        </button>
                    </div>
                </form>
            </div>
        </div>,
        document.body
    );
}
