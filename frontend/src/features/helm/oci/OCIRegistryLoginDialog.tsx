import React, { useState, useCallback } from 'react';
import { XMarkIcon, CloudIcon } from '@heroicons/react/24/outline';
import {
    LoginOCIRegistry,
    LoginACRWithAzureCLI
} from '../../../../wailsjs/go/main/App';
import { useNotification } from '../../../context';

export default function OCIRegistryLoginDialog({ onClose, onSuccess }) {
    const [registry, setRegistry] = useState('');
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [loading, setLoading] = useState(false);
    const { addNotification } = useNotification();

    const isACR = registry.includes('.azurecr.io');

    const handleSubmit = useCallback(async (e) => {
        e.preventDefault();
        if (!registry.trim()) return;

        setLoading(true);
        try {
            await LoginOCIRegistry(registry.trim(), username.trim(), password);
            addNotification({
                type: 'success',
                title: 'Logged in',
                message: `Successfully logged into ${registry}`
            });
            onSuccess();
        } catch (err) {
            console.error('Failed to login:', err);
            addNotification({
                type: 'error',
                title: 'Login failed',
                message: err?.message || String(err)
            });
        } finally {
            setLoading(false);
        }
    }, [registry, username, password, addNotification, onSuccess]);

    const handleAzureLogin = useCallback(async () => {
        if (!registry.trim()) return;

        setLoading(true);
        try {
            await LoginACRWithAzureCLI(registry.trim());
            addNotification({
                type: 'success',
                title: 'Logged in via Azure CLI',
                message: `Successfully logged into ${registry}`
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
            setLoading(false);
        }
    }, [registry, addNotification, onSuccess]);

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

                <form onSubmit={handleSubmit} className="p-4 space-y-4">
                    <div>
                        <label className="block text-sm font-medium text-gray-300 mb-1">
                            Registry URL
                        </label>
                        <input
                            type="text"
                            value={registry}
                            onChange={(e) => setRegistry(e.target.value)}
                            placeholder="e.g., myregistry.azurecr.io"
                            className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
                            autoFocus
                        />
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
                                disabled={loading || !registry.trim()}
                                className="w-full flex items-center justify-center gap-2 px-3 py-2 bg-blue-500/20 hover:bg-blue-500/30 text-blue-300 rounded-md transition-colors disabled:opacity-50"
                            >
                                <CloudIcon className="h-4 w-4" />
                                {loading ? 'Logging in...' : 'Login with Azure CLI'}
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
                                    value={username}
                                    onChange={(e) => setUsername(e.target.value)}
                                    className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
                                />
                            </div>

                            <div>
                                <label className="block text-sm font-medium text-gray-300 mb-1">
                                    Password / Token
                                </label>
                                <input
                                    type="password"
                                    value={password}
                                    onChange={(e) => setPassword(e.target.value)}
                                    className="w-full px-3 py-2 bg-background border border-border rounded-md text-text placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-primary focus:border-transparent"
                                />
                            </div>
                        </div>
                    </div>

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
                            disabled={loading || !registry.trim() || !username.trim() || !password}
                            className="px-4 py-2 text-sm bg-primary hover:bg-primary/80 text-white rounded-md transition-colors disabled:opacity-50"
                        >
                            {loading ? 'Logging in...' : 'Login'}
                        </button>
                    </div>
                </form>
            </div>
        </div>
    );
}
