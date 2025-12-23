import React, { useMemo, useState, useCallback, useEffect } from 'react';
import {
    EllipsisVerticalIcon,
    ArrowPathIcon,
    PlusIcon,
    TrashIcon,
    ArrowUpIcon,
    ArrowDownIcon
} from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import {
    ListHelmRepositories,
    RemoveHelmRepository,
    UpdateHelmRepository,
    UpdateAllHelmRepositories,
    SetHelmRepositoryPriority
} from '../../../../wailsjs/go/main/App';
import HelmRepoAddDialog from './HelmRepoAddDialog';
import { useNotification } from '../../../context/NotificationContext';

export default function HelmRepoList({ isVisible }) {
    const [repos, setRepos] = useState([]);
    const [loading, setLoading] = useState(false);
    const [showAddDialog, setShowAddDialog] = useState(false);
    const [updatingRepo, setUpdatingRepo] = useState(null);
    const { addNotification } = useNotification();

    const fetchRepos = useCallback(async () => {
        if (!isVisible) return;
        setLoading(true);
        try {
            const data = await ListHelmRepositories();
            setRepos(data || []);
        } catch (err) {
            console.error('Failed to fetch repositories:', err);
            addNotification({
                type: 'error',
                title: 'Failed to fetch repositories',
                message: err?.message || String(err)
            });
        } finally {
            setLoading(false);
        }
    }, [isVisible, addNotification]);

    useEffect(() => {
        fetchRepos();
    }, [fetchRepos]);

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

    const handleRemoveRepo = useCallback(async (repo) => {
        if (!window.confirm(`Are you sure you want to remove repository "${repo.name}"?`)) {
            return;
        }
        try {
            await RemoveHelmRepository(repo.name);
            addNotification({
                type: 'success',
                title: 'Repository removed',
                message: `Successfully removed ${repo.name}`
            });
            fetchRepos();
        } catch (err) {
            console.error('Failed to remove repository:', err);
            addNotification({
                type: 'error',
                title: 'Failed to remove repository',
                message: err?.message || String(err)
            });
        }
    }, [fetchRepos, addNotification]);

    const handleChangePriority = useCallback(async (repo, delta) => {
        const newPriority = Math.max(0, repo.priority + delta);
        try {
            await SetHelmRepositoryPriority(repo.name, newPriority);
            fetchRepos();
        } catch (err) {
            console.error('Failed to update priority:', err);
            addNotification({
                type: 'error',
                title: 'Failed to update priority',
                message: err?.message || String(err)
            });
        }
    }, [fetchRepos, addNotification]);

    const handleAddSuccess = useCallback(() => {
        setShowAddDialog(false);
        fetchRepos();
    }, [fetchRepos]);

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
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            width: '100px',
            render: (item) => (
                <div className="flex items-center justify-center gap-1">
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
                </div>
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [handleUpdateRepo, handleRemoveRepo, handleChangePriority, updatingRepo]);

    // Generate unique IDs for the list
    const dataWithIds = useMemo(() => {
        return repos.map(r => ({
            ...r,
            metadata: {
                uid: r.name,
                name: r.name
            }
        }));
    }, [repos]);

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
                className="flex items-center gap-1.5 px-3 py-1.5 text-sm bg-primary/20 hover:bg-primary/30 text-primary rounded-md transition-colors"
            >
                <PlusIcon className="h-4 w-4" />
                Add Repository
            </button>
        </div>
    );

    return (
        <>
            <ResourceList
                title="Helm Repositories"
                columns={columns}
                data={dataWithIds}
                isLoading={loading}
                showNamespaceSelector={false}
                initialSort={{ key: 'priority', direction: 'asc' }}
                resourceType="helmrepos"
                onRefresh={fetchRepos}
                customHeaderActions={customActions}
            />

            {showAddDialog && (
                <HelmRepoAddDialog
                    onClose={() => setShowAddDialog(false)}
                    onSuccess={handleAddSuccess}
                />
            )}
        </>
    );
}
