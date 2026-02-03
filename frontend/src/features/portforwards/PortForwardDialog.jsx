import React, { useState, useEffect, useCallback } from 'react';
import { createPortal } from 'react-dom';
import { XMarkIcon, ArrowPathIcon } from '@heroicons/react/24/outline';
import { GetRandomAvailablePort, GetPodPorts, GetServicePorts } from '../../../wailsjs/go/main/App';

export default function PortForwardDialog({
    open,
    onOpenChange,
    config,
    onSave,
    contexts,
    currentContext
}) {
    const [formData, setFormData] = useState({
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
        autoStart: true,
        openInBrowser: false
    });
    const [error, setError] = useState('');
    const [saving, setSaving] = useState(false);
    const [availablePorts, setAvailablePorts] = useState([]);
    const [loadingPorts, setLoadingPorts] = useState(false);

    const isEditing = !!config;

    // Initialize form when dialog opens
    useEffect(() => {
        if (open) {
            if (config) {
                // Editing existing config
                setFormData({
                    id: config.id || '',
                    context: config.context || currentContext || '',
                    namespace: config.namespace || '',
                    resourceType: config.resourceType || 'pod',
                    resourceName: config.resourceName || '',
                    localPort: config.localPort || 0,
                    remotePort: config.remotePort || 0,
                    label: config.label || '',
                    favorite: config.favorite || false,
                    https: config.https || false
                });
                setAvailablePorts([]);
            } else {
                // New config
                setFormData({
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
                    autoStart: true,
                    openInBrowser: false
                });
                // Get a random available port
                GetRandomAvailablePort()
                    .then(port => setFormData(prev => ({ ...prev, localPort: port })))
                    .catch(console.error);
                setAvailablePorts([]);
            }
            setError('');
        }
    }, [open, config, currentContext]);

    // Fetch available ports when resource changes
    const fetchResourcePorts = useCallback(async () => {
        if (!formData.namespace || !formData.resourceName) {
            setAvailablePorts([]);
            return;
        }

        setLoadingPorts(true);
        try {
            let ports;
            if (formData.resourceType === 'pod') {
                ports = await GetPodPorts(formData.namespace, formData.resourceName);
            } else {
                ports = await GetServicePorts(formData.namespace, formData.resourceName);
            }
            setAvailablePorts(ports || []);
            // Auto-select first port if none selected
            if (ports && ports.length > 0 && !formData.remotePort) {
                setFormData(prev => ({ ...prev, remotePort: ports[0] }));
            }
        } catch (err) {
            console.error('Failed to fetch ports:', err);
            setAvailablePorts([]);
        } finally {
            setLoadingPorts(false);
        }
    }, [formData.namespace, formData.resourceName, formData.resourceType]);

    useEffect(() => {
        if (open && formData.resourceName) {
            fetchResourcePorts();
        }
    }, [open, formData.resourceName, fetchResourcePorts]);

    const handleChange = (field, value) => {
        setFormData(prev => ({ ...prev, [field]: value }));
        setError('');
    };

    const handleSubmit = async (e) => {
        e.preventDefault();
        setError('');

        // Validation
        if (!formData.context) {
            setError('Context is required');
            return;
        }
        if (!formData.namespace) {
            setError('Namespace is required');
            return;
        }
        if (!formData.resourceName) {
            setError('Resource name is required');
            return;
        }
        if (!formData.localPort || formData.localPort < 1 || formData.localPort > 65535) {
            setError('Local port must be between 1 and 65535');
            return;
        }
        if (!formData.remotePort || formData.remotePort < 1 || formData.remotePort > 65535) {
            setError('Remote port must be between 1 and 65535');
            return;
        }

        setSaving(true);
        try {
            await onSave(formData);
        } catch (err) {
            setError(err.message || 'Failed to save port forward');
        } finally {
            setSaving(false);
        }
    };

    const handleClose = () => {
        onOpenChange(false);
    };

    const randomizePort = useCallback(async () => {
        try {
            const port = await GetRandomAvailablePort();
            setFormData(prev => ({ ...prev, localPort: port }));
        } catch (err) {
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

                <form onSubmit={handleSubmit} className="space-y-4">
                    {isEditing ? (
                        /* Edit mode - show resource info as read-only */
                        <>
                            {/* Resource Info */}
                            <div className="bg-surface-light rounded-lg p-3 space-y-2">
                                <div className="flex justify-between text-sm">
                                    <span className="text-gray-500">Resource</span>
                                    <span className="text-gray-200 capitalize">{formData.resourceType}: {formData.resourceName}</span>
                                </div>
                                <div className="flex justify-between text-sm">
                                    <span className="text-gray-500">Namespace</span>
                                    <span className="text-gray-200">{formData.namespace}</span>
                                </div>
                                <div className="flex justify-between text-sm">
                                    <span className="text-gray-500">Remote Port</span>
                                    <span className="text-gray-200 font-mono">{formData.remotePort}</span>
                                </div>
                            </div>

                            {/* Label */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    Label (optional)
                                </label>
                                <input
                                    type="text"
                                    value={formData.label}
                                    onChange={(e) => handleChange('label', e.target.value)}
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
                                    value={formData.localPort || ''}
                                    onChange={(e) => handleChange('localPort', parseInt(e.target.value) || 0)}
                                    placeholder="8080"
                                    min="1"
                                    max="65535"
                                    className="w-full"
                                />
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
                                    value={formData.label}
                                    onChange={(e) => handleChange('label', e.target.value)}
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
                                    value={formData.context}
                                    onChange={(e) => handleChange('context', e.target.value)}
                                    className="w-full"
                                >
                                    <option value="">Select context...</option>
                                    {contexts.map(ctx => (
                                        <option key={ctx} value={ctx}>{ctx}</option>
                                    ))}
                                </select>
                            </div>

                            {/* Namespace */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    Namespace
                                </label>
                                <input
                                    type="text"
                                    value={formData.namespace}
                                    onChange={(e) => handleChange('namespace', e.target.value)}
                                    placeholder="default"
                                    className="w-full"
                                />
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
                                            checked={formData.resourceType === 'pod'}
                                            onChange={(e) => handleChange('resourceType', e.target.value)}
                                        />
                                        <span>Pod</span>
                                    </label>
                                    <label className="flex items-center gap-2">
                                        <input
                                            type="radio"
                                            name="resourceType"
                                            value="service"
                                            checked={formData.resourceType === 'service'}
                                            onChange={(e) => handleChange('resourceType', e.target.value)}
                                        />
                                        <span>Service</span>
                                    </label>
                                </div>
                            </div>

                            {/* Resource Name */}
                            <div>
                                <label className="block font-medium text-gray-300 mb-1">
                                    {formData.resourceType === 'pod' ? 'Pod Name' : 'Service Name'}
                                </label>
                                <input
                                    type="text"
                                    value={formData.resourceName}
                                    onChange={(e) => handleChange('resourceName', e.target.value)}
                                    placeholder={formData.resourceType === 'pod' ? 'my-pod-abc123' : 'my-service'}
                                    className="w-full"
                                />
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
                                value={formData.localPort || ''}
                                onChange={(e) => handleChange('localPort', parseInt(e.target.value) || 0)}
                                placeholder="8080"
                                min="1"
                                max="65535"
                                className="w-full"
                            />
                        </div>
                        <div>
                            <label className="block font-medium text-gray-300 mb-1">
                                Remote Port
                                {loadingPorts && <span className="ml-2 text-xs text-gray-500">Loading...</span>}
                            </label>
                            {availablePorts.length > 0 ? (
                                <select
                                    value={formData.remotePort || ''}
                                    onChange={(e) => handleChange('remotePort', parseInt(e.target.value) || 0)}
                                    className="w-full"
                                >
                                    <option value="">Select port...</option>
                                    {availablePorts.map(port => (
                                        <option key={port} value={port}>{port}</option>
                                    ))}
                                </select>
                            ) : (
                                <input
                                    type="number"
                                    value={formData.remotePort || ''}
                                    onChange={(e) => handleChange('remotePort', parseInt(e.target.value) || 0)}
                                    placeholder="80"
                                    min="1"
                                    max="65535"
                                    className="w-full"
                                />
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
                                        checked={formData.autoStart}
                                        onChange={(e) => handleChange('autoStart', e.target.checked)}
                                    />
                                    <span className="text-gray-300">Start immediately</span>
                                </label>
                                <label className={`flex items-center gap-2 ${!formData.autoStart && 'opacity-50'}`}>
                                    <input
                                        type="checkbox"
                                        checked={formData.openInBrowser}
                                        onChange={(e) => handleChange('openInBrowser', e.target.checked)}
                                        disabled={!formData.autoStart}
                                    />
                                    <span className="text-gray-300">Open in browser</span>
                                </label>
                            </>
                        )}
                        <div className="flex gap-6">
                            <label className="flex items-center gap-2">
                                <input
                                    type="checkbox"
                                    checked={formData.https}
                                    onChange={(e) => handleChange('https', e.target.checked)}
                                />
                                <span className="text-gray-300">Use HTTPS</span>
                            </label>
                            <label className="flex items-center gap-2">
                                <input
                                    type="checkbox"
                                    checked={formData.favorite}
                                    onChange={(e) => handleChange('favorite', e.target.checked)}
                                />
                                <span className="text-gray-300">Favorite</span>
                            </label>
                        </div>
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
                            {saving ? 'Saving...' : isEditing ? 'Save Changes' : 'Add Port Forward'}
                        </button>
                    </div>
                </form>
            </div>
        </div>,
        document.body
    );
}
