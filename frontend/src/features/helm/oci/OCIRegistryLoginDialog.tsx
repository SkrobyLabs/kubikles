import React, { useState, useCallback } from 'react';
import { XMarkIcon, CloudIcon } from '@heroicons/react/24/outline';
import {
    LoginOCIRegistry,
    LoginACRWithAzureCLI
} from 'wailsjs/go/main/App';
import { useNotification } from '~/context';
import { useForm, ociBasicLoginSchema } from '~/lib/validation';

export default function OCIRegistryLoginDialog({ onClose, onSuccess }) {
    const { addNotification } = useNotification();
    const [azureLoading, setAzureLoading] = useState(false);

    const form = useForm({
        schema: ociBasicLoginSchema,
        initialValues: { registry: '', username: '', password: '' },
        onSubmit: async (values) => {
            await LoginOCIRegistry(values.registry.trim(), values.username.trim(), values.password);
            addNotification({
                type: 'success',
                title: 'Logged in',
                message: `Successfully logged into ${values.registry}`
            });
            onSuccess();
        },
    });

    const isACR = form.values.registry.includes('.azurecr.io');

    const handleAzureLogin = useCallback(async () => {
        if (!form.values.registry.trim()) return;

        setAzureLoading(true);
        try {
            await LoginACRWithAzureCLI(form.values.registry.trim());
            addNotification({
                type: 'success',
                title: 'Logged in via Azure CLI',
                message: `Successfully logged into ${form.values.registry}`
            });
            onSuccess();
        } catch (err) {
            console.error('Failed to login via Azure CLI:', err);
            addNotification({
                type: 'error',
                title: 'Azure login failed',
                message: err?.message || String(err)
            });
        } finally {
            setAzureLoading(false);
        }
    }, [form.values.registry, addNotification, onSuccess]);

    const loading = form.isSubmitting || azureLoading;

    return (
        <div className="fixed inset-0 bg-black/50 flex items-center justify-center z-50" onClick={onClose}>
            <div
                className="bg-surface border border-border rounded-lg shadow-xl w-full max-w-md"
                onClick={e => e.stopPropagation()}
            >
                <div className="flex items-center justify-between px-4 py-3 border-b border-border">
                    <h2 className="text-lg font-semibold">Login to OCI Registry</h2>
                    <button
                        onClick={onClose}
                        className="p-1 text-gray-400 hover:text-white rounded transition-colors"
                    >
                        <XMarkIcon className="h-5 w-5" />
                    </button>
                </div>

                <form onSubmit={form.handleSubmit} className="p-4 space-y-4">
                    <div>
                        <label className="block text-sm font-medium text-gray-300 mb-1">
                            Registry URL
                        </label>
                        <input
                            type="text"
                            {...form.getFieldProps('registry')}
                            placeholder="e.g., myregistry.azurecr.io"
                            className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
                            autoFocus
                        />
                        {form.touched.registry && form.errors.registry && (
                            <span className="text-red-400 text-xs mt-1 block">{form.errors.registry}</span>
                        )}
                        {isACR && (
                            <p className="mt-1 text-xs text-blue-400 flex items-center gap-1">
                                <CloudIcon className="h-3 w-3" />
                                Azure Container Registry detected
                            </p>
                        )}
                    </div>

                    {isACR && (
                        <div className="bg-blue-500/10 border border-blue-500/30 rounded-md p-3">
                            <p className="text-sm text-blue-300 mb-2">
                                For Azure Container Registry, you can login using your Azure CLI credentials:
                            </p>
                            <button
                                type="button"
                                onClick={handleAzureLogin}
                                disabled={loading || !form.values.registry.trim()}
                                className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-blue-500/20 hover:bg-blue-500/30 text-blue-300 rounded-md transition-colors disabled:opacity-50"
                            >
                                <CloudIcon className="h-4 w-4" />
                                {azureLoading ? 'Logging in...' : 'Login with Azure CLI'}
                            </button>
                        </div>
                    )}

                    <div className="border-t border-border pt-4">
                        <p className="text-xs text-gray-500 mb-3">
                            {isACR ? 'Or login with username and password:' : 'Login with username and password:'}
                        </p>

                        <div className="space-y-3">
                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-1">
                                    Username
                                </label>
                                <input
                                    type="text"
                                    {...form.getFieldProps('username')}
                                    className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
                                />
                                {form.touched.username && form.errors.username && (
                                    <span className="text-red-400 text-xs mt-1 block">{form.errors.username}</span>
                                )}
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-1">
                                    Password / Token
                                </label>
                                <input
                                    type="password"
                                    {...form.getFieldProps('password')}
                                    className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
                                />
                                {form.touched.password && form.errors.password && (
                                    <span className="text-red-400 text-xs mt-1 block">{form.errors.password}</span>
                                )}
                            </div>
                        </div>
                    </div>

                    {form.submitError && (
                        <div className="p-3 bg-red-500/10 border border-red-500/30 rounded-md text-red-400 text-sm">
                            {form.submitError}
                        </div>
                    )}

                    <div className="flex justify-end gap-2 pt-2">
                        <button
                            type="button"
                            onClick={onClose}
                            className="px-4 py-2 text-sm text-gray-400 hover:text-white transition-colors"
                        >
                            Cancel
                        </button>
                        <button
                            type="submit"
                            disabled={form.isSubmitting || !form.isValid}
                            className="px-4 py-2 text-sm bg-primary hover:bg-primary/80 text-white rounded-md transition-colors disabled:opacity-50"
                        >
                            {form.isSubmitting ? 'Logging in...' : 'Login'}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
}
