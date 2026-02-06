import React, { useMemo, useState, useCallback, useEffect } from 'react';
import {
    EllipsisVerticalIcon,
    ArrowPathIcon,
    PlusIcon,
    TrashIcon,
    ArrowUpIcon,
    ArrowDownIcon,
    CloudIcon,
    ServerIcon,
    ArrowRightStartOnRectangleIcon,
    ArrowLeftOnRectangleIcon,
    CheckCircleIcon,
    XCircleIcon
} from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import {
    ListHelmRepositories,
    RemoveHelmRepository,
    UpdateHelmRepository,
    UpdateAllHelmRepositories,
    SetHelmRepositoryPriority,
    ListOCIRegistries,
    LoginACRWithAzureCLI,
    LogoutOCIRegistry,
    SetOCIRegistryPriority,
    RemoveOCIRegistry
} from 'wailsjs/go/main/App';
import HelmRepoAddDialog from './HelmRepoAddDialog';
import { OCIRegistryLoginDialog } from '../oci';
import { useNotification } from '~/context';
import { useUI } from '~/context';

export default function HelmRepoList({ isVisible }) {
    const [repos, setRepos] = useState([]);
    const [ociRegistries, setOciRegistries] = useState([]);
    const [loading, setLoading] = useState(false);
    const [showAddDialog, setShowAddDialog] = useState(false);
    const [showOCILoginDialog, setShowOCILoginDialog] = useState(false);
    const [updatingRepo, setUpdatingRepo] = useState(null);
    const [loggingIn, setLoggingIn] = useState(null);
    const { addNotification } = useNotification();
    const { openModal, closeModal } = useUI();

    const fetchData = useCallback(async () => {
        if (!isVisible) return;
        setLoading(true);
        try {
            const [repoData, ociData] = await Promise.all([
                ListHelmRepositories(),
                ListOCIRegistries()
            ]);
            setRepos(repoData || []);
            setOciRegistries(ociData || []);
        } catch (err) {
            console.error('Failed to fetch chart sources:', err);
            addNotification({
                type: 'error',
                title: 'Failed to fetch chart sources',
                message: err?.message || String(err)
            });
        } finally {
            setLoading(false);
        }
    }, [isVisible, addNotification]);

    useEffect(() => {
        fetchData();
    }, [fetchData]);

    const handleUpdateRepo = useCallback(async (repo) => {
        setUpdatingRepo(repo.name);
        try {
            await UpdateHelmRepository(repo.name);
            addNotification({
                type: 'success',
                title: 'Repository updated',
                message: `Successfully updated index for ${repo.name}`
            });
        } catch (err) {
            console.error('Failed to update repository:', err);
            addNotification({
                type: 'error',
                title: 'Failed to update repository',
                message: err?.message || String(err)
            });
        } finally {
            setUpdatingRepo(null);
        }
    }, [addNotification]);

    const handleUpdateAll = useCallback(async () => {
        setLoading(true);
        try {
            await UpdateAllHelmRepositories();
            addNotification({
                type: 'success',
                title: 'Repositories updated',
                message: 'Successfully updated all repository indexes'
            });
        } catch (err) {
            console.error('Failed to update repositories:', err);
            addNotification({
                type: 'error',
                title: 'Failed to update repositories',
                message: err?.message || String(err)
            });
        } finally {
            setLoading(false);
        }
    }, [addNotification]);

    const handleRemoveRepo = useCallback((repo) => {
        openModal({
            title: `Remove ${repo.name}`,
            content: `Are you sure you want to remove repository "${repo.name}"?`,
            confirmText: 'Remove',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await RemoveHelmRepository(repo.name);
                    addNotification({
                        type: 'success',
                        title: 'Repository removed',
                        message: `Successfully removed ${repo.name}`
                    });
                    closeModal();
                    fetchData();
                } catch (err) {
                    console.error('Failed to remove repository:', err);
                    addNotification({
                        type: 'error',
                        title: 'Failed to remove repository',
                        message: err?.message || String(err)
                    });
                }
            }
        });
    }, [openModal, closeModal, fetchData, addNotification]);

    const handleChangePriority = useCallback(async (item, delta) => {
        const newPriority = Math.max(0, item.priority + delta);
        try {
            if (item.type === 'oci') {
                await SetOCIRegistryPriority(item.url, newPriority);
            } else {
                await SetHelmRepositoryPriority(item.name, newPriority);
            }
            fetchData();
        } catch (err) {
            console.error('Failed to update priority:', err);
            addNotification({
                type: 'error',
                title: 'Failed to update priority',
                message: err?.message || String(err)
            });
        }
    }, [fetchData, addNotification]);

    const handleACRLogin = useCallback(async (registry) => {
        setLoggingIn(registry.url);
        try {
            await LoginACRWithAzureCLI(registry.url);
            addNotification({
                type: 'success',
                title: 'Logged in to ACR',
                message: `Successfully authenticated to ${registry.url}`
            });
            fetchData();
        } catch (err) {
            console.error('Failed to login to ACR:', err);
            addNotification({
                type: 'error',
                title: 'Failed to login to ACR',
                message: err?.message || String(err)
            });
        } finally {
            setLoggingIn(null);
        }
    }, [addNotification, fetchData]);

    const handleOCILogout = useCallback((registry) => {
        openModal({
            title: `Logout from ${registry.url}`,
            content: `Are you sure you want to logout from "${registry.url}"? You will need to re-authenticate to pull charts from this registry.`,
            confirmText: 'Logout',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await LogoutOCIRegistry(registry.url);
                    addNotification({
                        type: 'success',
                        title: 'Logged out',
                        message: `Successfully logged out from ${registry.url}`
                    });
                    closeModal();
                    fetchData();
                } catch (err) {
                    addNotification({
                        type: 'error',
                        title: 'Failed to logout',
                        message: err?.message || String(err)
                    });
                }
            }
        });
    }, [openModal, closeModal, addNotification, fetchData]);

    const handleOCIRemove = useCallback((registry) => {
        openModal({
            title: `Remove ${registry.name}`,
            content: `Are you sure you want to remove "${registry.url}"? This will logout and remove the registry from the list.`,
            confirmText: 'Remove',
            confirmStyle: 'danger',
            onConfirm: async () => {
                try {
                    await RemoveOCIRegistry(registry.url);
                    addNotification({
                        type: 'success',
                        title: 'Registry removed',
                        message: `Successfully removed ${registry.url}`
                    });
                    closeModal();
                    fetchData();
                } catch (err) {
                    addNotification({
                        type: 'error',
                        title: 'Failed to remove registry',
                        message: err?.message || String(err)
                    });
                }
            }
        });
    }, [openModal, closeModal, addNotification, fetchData]);

    const handleAddSuccess = useCallback(() => {
        setShowAddDialog(false);
        fetchData();
    }, [fetchData]);

    const handleOCILoginSuccess = useCallback(() => {
        setShowOCILoginDialog(false);
        fetchData();
    }, [fetchData]);

    // Combine repos and OCI registries into a single list
    const combinedData = useMemo(() => {
        const httpRepos = repos.map(r => ({
            ...r,
            type: 'http',
            authenticated: true, // HTTP repos are always "authenticated" if they exist
            metadata: {
                uid: `http-${r.name}`,
                name: r.name
            }
        }));

        const ociRepos = ociRegistries.map(r => ({
            name: r.url.replace(/^https?:\/\//, '').split('/')[0], // Extract hostname as name
            url: r.url,
            priority: r.priority,
            type: 'oci',
            isAcr: r.isAcr,
            authenticated: r.authenticated,
            username: r.username,
            metadata: {
                uid: `oci-${r.url}`,
                name: r.url
            }
        }));

        return [...httpRepos, ...ociRepos];
    }, [repos, ociRegistries]);

    const columns = useMemo(() => [
        {
            key: 'priority',
            label: '#',
            width: '60px',
            render: (item) => (
                <div className="flex items-center gap-1">
                    <span className="text-gray-400 font-mono text-xs">{item.priority}</span>
                    <div className="flex flex-col">
                        <button
                            onClick={(e) => { e.stopPropagation(); handleChangePriority(item, -10); }}
                            className="p-0.5 text-gray-500 hover:text-white transition-colors"
                            title="Increase priority (lower number)"
                        >
                            <ArrowUpIcon className="h-3 w-3" />
                        </button>
                        <button
                            onClick={(e) => { e.stopPropagation(); handleChangePriority(item, 10); }}
                            className="p-0.5 text-gray-500 hover:text-white transition-colors"
                            title="Decrease priority (higher number)"
                        >
                            <ArrowDownIcon className="h-3 w-3" />
                        </button>
                    </div>
                </div>
            ),
            getValue: (item) => item.priority,
            initialSort: 'asc'
        },
        {
            key: 'type',
            label: 'Type',
            width: '80px',
            render: (item) => (
                <div className="flex items-center gap-1.5">
                    {item.type === 'oci' ? (
                        <>
                            {item.isAcr ? (
                                <CloudIcon className="h-4 w-4 text-blue-400" title="Azure Container Registry" />
                            ) : (
                                <CloudIcon className="h-4 w-4 text-purple-400" title="OCI Registry" />
                            )}
                            <span className="text-xs text-gray-400">OCI</span>
                        </>
                    ) : (
                        <>
                            <ServerIcon className="h-4 w-4 text-gray-400" />
                            <span className="text-xs text-gray-400">HTTP</span>
                        </>
                    )}
                </div>
            ),
            getValue: (item) => item.type
        },
        {
            key: 'name',
            label: 'Name',
            render: (item) => (
                <span className="font-medium">{item.name}</span>
            ),
            getValue: (item) => item.name
        },
        {
            key: 'url',
            label: 'URL',
            render: (item) => (
                <span className="font-mono text-xs text-gray-400 truncate block max-w-md" title={item.url}>
                    {item.url}
                </span>
            ),
            getValue: (item) => item.url
        },
        {
            key: 'status',
            label: 'Status',
            width: '120px',
            render: (item) => {
                if (item.type === 'http') {
                    return <span className="text-xs text-gray-500">-</span>;
                }
                return (
                    <div className="flex items-center gap-1.5">
                        {item.authenticated ? (
                            <>
                                <CheckCircleIcon className="h-4 w-4 text-green-400" />
                                <span className="text-green-400 text-xs">Authenticated</span>
                            </>
                        ) : (
                            <>
                                <XCircleIcon className="h-4 w-4 text-gray-500" />
                                <span className="text-gray-500 text-xs">Not logged in</span>
                            </>
                        )}
                    </div>
                );
            },
            getValue: (item) => item.authenticated ? 1 : 0
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            width: '140px',
            render: (item) => (
                <div className="flex items-center justify-center gap-1">
                    {item.type === 'http' ? (
                        // HTTP repo actions
                        <>
                            <button
                                onClick={(e) => { e.stopPropagation(); handleUpdateRepo(item); }}
                                disabled={updatingRepo === item.name}
                                className="p-1.5 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors disabled:opacity-50"
                                title="Update repository index"
                            >
                                <ArrowPathIcon className={`h-4 w-4 ${updatingRepo === item.name ? 'animate-spin' : ''}`} />
                            </button>
                            <button
                                onClick={(e) => { e.stopPropagation(); handleRemoveRepo(item); }}
                                className="p-1.5 text-gray-400 hover:text-red-400 hover:bg-white/10 rounded transition-colors"
                                title="Remove repository"
                            >
                                <TrashIcon className="h-4 w-4" />
                            </button>
                        </>
                    ) : (
                        // OCI registry actions
                        <>
                            {item.isAcr && !item.authenticated && (
                                <button
                                    onClick={(e) => { e.stopPropagation(); handleACRLogin(item); }}
                                    disabled={loggingIn === item.url}
                                    className="p-1.5 text-blue-400 hover:text-blue-300 hover:bg-blue-400/10 rounded transition-colors disabled:opacity-50"
                                    title="Login with Azure CLI"
                                >
                                    {loggingIn === item.url ? (
                                        <ArrowPathIcon className="h-4 w-4 animate-spin" />
                                    ) : (
                                        <ArrowLeftOnRectangleIcon className="h-4 w-4" />
                                    )}
                                </button>
                            )}
                            {item.authenticated && (
                                <button
                                    onClick={(e) => { e.stopPropagation(); handleOCILogout(item); }}
                                    className="p-1.5 text-gray-400 hover:text-yellow-400 hover:bg-white/10 rounded transition-colors"
                                    title="Logout"
                                >
                                    <ArrowRightStartOnRectangleIcon className="h-4 w-4" />
                                </button>
                            )}
                            <button
                                onClick={(e) => { e.stopPropagation(); handleOCIRemove(item); }}
                                className="p-1.5 text-gray-400 hover:text-red-400 hover:bg-white/10 rounded transition-colors"
                                title="Remove registry"
                            >
                                <TrashIcon className="h-4 w-4" />
                            </button>
                        </>
                    )}
                </div>
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleUpdateRepo, handleRemoveRepo, handleChangePriority, handleACRLogin, handleOCILogout, handleOCIRemove, updatingRepo, loggingIn]);

    const customActions = (
        <div className="flex items-center gap-2">
            <button
                onClick={handleUpdateAll}
                disabled={loading}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-surface hover:bg-white/10 border border-border rounded-md transition-colors disabled:opacity-50"
                title="Update all repository indexes"
            >
                <ArrowPathIcon className={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
                Update All
            </button>
            <button
                onClick={() => setShowAddDialog(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-surface hover:bg-white/10 border border-border rounded-md transition-colors"
            >
                <PlusIcon className="h-4 w-4" />
                Add Repository
            </button>
            <button
                onClick={() => setShowOCILoginDialog(true)}
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-primary/20 hover:bg-primary/30 text-primary rounded-md transition-colors"
            >
                <CloudIcon className="h-4 w-4" />
                Login to Registry
            </button>
        </div>
    );

    return (
        <>
            <ResourceList
                title="Chart Sources"
                columns={columns}
                data={combinedData}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'priority', direction: 'asc' }}
                resourceType="chartsources"
                onRefresh={fetchData}
                customHeaderActions={customActions}
            />

            {showAddDialog && (
                <HelmRepoAddDialog
                    onClose={() => setShowAddDialog(false)}
                    onSuccess={handleAddSuccess}
                />
            )}

            {showOCILoginDialog && (
                <OCIRegistryLoginDialog
                    onClose={() => setShowOCILoginDialog(false)}
                    onSuccess={handleOCILoginSuccess}
                />
            )}
        </>
    );
}
