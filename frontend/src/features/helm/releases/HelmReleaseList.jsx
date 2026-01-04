import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, ArrowPathIcon, CheckCircleIcon, ExclamationTriangleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import BulkActionModal from '../../../components/shared/BulkActionModal';
import HelmReleaseActionsMenu from './HelmReleaseActionsMenu';
import HelmUpgradeDialog from './HelmUpgradeDialog';
import { useHelmReleases } from '../../../hooks/useHelmReleases';
import { useHelmReleaseActions } from './useHelmReleaseActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import { useNotification } from '../../../context/NotificationContext';
import { useSelection } from '../../../hooks/useSelection';
import { ForceHelmReleaseStatus, UninstallHelmRelease, GetHelmReleaseValues, SaveYamlBackup } from '../../../../wailsjs/go/main/App';
import Logger from '../../../utils/Logger';

const formatAge = (timestamp) => {
    if (!timestamp) return '-';
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now - date;

    const seconds = Math.floor(diff / 1000);
    if (seconds < 60) return `${seconds}s`;

    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m`;

    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h`;

    const days = Math.floor(hours / 24);
    return `${days}d`;
};

const getStatusIcon = (status) => {
    const statusLower = status?.toLowerCase() || '';
    if (statusLower === 'deployed') {
        return <CheckCircleIcon className="h-4 w-4 text-green-400" />;
    } else if (statusLower === 'failed') {
        return <XCircleIcon className="h-4 w-4 text-red-400" />;
    } else if (statusLower === 'pending-install' || statusLower === 'pending-upgrade' || statusLower === 'pending-rollback') {
        return <ArrowPathIcon className="h-4 w-4 text-yellow-400 animate-spin" />;
    } else if (statusLower === 'superseded' || statusLower === 'uninstalling') {
        return <ExclamationTriangleIcon className="h-4 w-4 text-gray-400" />;
    }
    return null;
};

const getStatusClass = (status) => {
    const statusLower = status?.toLowerCase() || '';
    if (statusLower === 'deployed') return 'text-green-400';
    if (statusLower === 'failed') return 'text-red-400';
    if (statusLower.includes('pending')) return 'text-yellow-400';
    return 'text-gray-400';
};

export default function HelmReleaseList({ isVisible }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { activeMenuId, setActiveMenuId, openModal, closeModal } = useUI();
    const { addNotification } = useNotification();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });
    const [upgradeRelease, setUpgradeRelease] = useState(null);
    const selection = useSelection();

    const [bulkActionModal, setBulkActionModal] = useState({ isOpen: false, action: null, items: [] });
    const [bulkProgress, setBulkProgress] = useState({ current: 0, total: 0, status: 'idle', results: [] });

    const { releases, loading, refresh } = useHelmReleases(currentContext, selectedNamespaces, isVisible);
    const {
        handleOpenDetails,
        handleViewValues,
        handleViewHistory,
        handleRollback,
        handleUninstall
    } = useHelmReleaseActions();

    const handleBulkDeleteClick = useCallback((selectedItems) => {
        setBulkActionModal({ isOpen: true, action: 'delete', items: selectedItems });
        setBulkProgress({ current: 0, total: selectedItems.length, status: 'idle', results: [] });
    }, []);

    const handleBulkActionConfirm = useCallback(async (items) => {
        Logger.info('Bulk uninstall started', { count: items.length });
        setBulkProgress(prev => ({ ...prev, status: 'inProgress', results: [] }));
        const results = [];
        for (let i = 0; i < items.length; i++) {
            const item = items[i];
            const namespace = item.namespace;
            const name = item.name;
            try {
                await UninstallHelmRelease(namespace, name);
                results.push({ name, namespace, success: true, message: '' });
            } catch (err) {
                results.push({ name, namespace, success: false, message: err.toString() });
            }
            setBulkProgress(prev => ({ ...prev, current: i + 1, results: [...results] }));
        }
        setBulkProgress(prev => ({ ...prev, status: 'complete' }));
        refresh();
    }, [refresh]);

    const handleBulkActionClose = useCallback(() => {
        setBulkActionModal({ isOpen: false, action: null, items: [] });
        setBulkProgress({ current: 0, total: 0, status: 'idle', results: [] });
    }, []);

    const handleExportYaml = useCallback(async (items) => {
        const entries = [];
        for (const item of items) {
            try {
                const values = await GetHelmReleaseValues(item.namespace, item.name);
                entries.push({ namespace: item.namespace, name: item.name, kind: 'HelmRelease', yaml: `# Helm Release: ${item.name}\n# Chart: ${item.chart}-${item.chartVersion}\n# Values:\n${values}` });
            } catch (err) {
                entries.push({ namespace: item.namespace, name: item.name, kind: 'HelmRelease', yaml: `# Failed: ${err}` });
            }
        }
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `helmreleases-backup-${timestamp}.zip`); } catch (err) { if (err?.toString()) alert('Failed: ' + err); }
    }, []);

    const handleUpgrade = useCallback((release) => {
        setUpgradeRelease(release);
    }, []);

    const handleForceStatus = useCallback((release) => {
        openModal({
            title: `Force Status: ${release.name}`,
            content: `Force release "${release.name}" status to "deployed"? This will mark the release as successfully deployed without making any changes to the actual resources.`,
            confirmText: 'Force Deployed',
            confirmStyle: 'primary',
            onConfirm: async () => {
                try {
                    await ForceHelmReleaseStatus(release.namespace, release.name, 'deployed');
                    addNotification({
                        type: 'success',
                        title: 'Status updated',
                        message: `Release "${release.name}" marked as deployed`
                    });
                    closeModal();
                    refresh();
                } catch (err) {
                    addNotification({
                        type: 'error',
                        title: 'Failed to update status',
                        message: err?.message || String(err)
                    });
                }
            }
        });
    }, [openModal, closeModal, addNotification, refresh]);

    const handleUpgradeSuccess = useCallback(() => {
        setUpgradeRelease(null);
        refresh();
    }, [refresh]);

    const handleRowClick = useCallback((item) => {
        handleOpenDetails(item);
    }, [handleOpenDetails]);

    const handleMenuOpenChange = useCallback((isOpen, menuId, buttonElement) => {
        if (isOpen && buttonElement) {
            const rect = buttonElement.getBoundingClientRect();
            setMenuPosition({
                top: rect.bottom + 4,
                left: rect.right - 192
            });
        }
        setActiveMenuId(isOpen ? menuId : null);
    }, [setActiveMenuId]);

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item) => item.name,
            getValue: (item) => item.name,
            initialSort: 'asc'
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (item) => item.namespace,
            getValue: (item) => item.namespace
        },
        {
            key: 'revision',
            label: 'Rev',
            render: (item) => item.revision,
            getValue: (item) => item.revision
        },
        {
            key: 'status',
            label: 'Status',
            render: (item) => (
                <div className="flex items-center gap-1.5">
                    {getStatusIcon(item.status)}
                    <span className={getStatusClass(item.status)}>{item.status}</span>
                </div>
            ),
            getValue: (item) => item.status
        },
        {
            key: 'chart',
            label: 'Chart',
            render: (item) => (
                <span className="font-mono text-xs">
                    {item.chart}
                    {item.chartVersion && <span className="text-gray-500">-{item.chartVersion}</span>}
                </span>
            ),
            getValue: (item) => `${item.chart}-${item.chartVersion || ''}`
        },
        {
            key: 'appVersion',
            label: 'App Version',
            render: (item) => item.appVersion || '-',
            getValue: (item) => item.appVersion || ''
        },
        {
            key: 'updated',
            label: 'Updated',
            render: (item) => formatAge(item.updated),
            getValue: (item) => item.updated
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item) => (
                <HelmReleaseActionsMenu
                    release={item}
                    isOpen={activeMenuId === `helm-${item.namespace}-${item.name}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen, buttonElement) => handleMenuOpenChange(isOpen, `helm-${item.namespace}-${item.name}`, buttonElement)}
                    onViewDetails={() => handleOpenDetails(item)}
                    onViewValues={() => handleViewValues(item)}
                    onViewHistory={() => handleViewHistory(item)}
                    onRollback={() => handleRollback(item)}
                    onUpgrade={() => handleUpgrade(item)}
                    onForceStatus={() => handleForceStatus(item)}
                    onUninstall={() => handleUninstall(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleOpenDetails, handleViewValues, handleViewHistory, handleRollback, handleUpgrade, handleForceStatus, handleUninstall]);

    // Generate a unique ID for each release since Helm releases don't have UIDs
    const dataWithIds = useMemo(() => {
        return releases.map(r => ({
            ...r,
            metadata: {
                uid: `${r.namespace}-${r.name}`,
                name: r.name,
                namespace: r.namespace
            }
        }));
    }, [releases]);

    return (
        <>
            <ResourceList
                title="Helm Releases"
                columns={columns}
                data={dataWithIds}
                isLoading={loading}
                namespaces={namespaces}
                currentNamespace={selectedNamespaces}
                onNamespaceChange={setSelectedNamespaces}
                showNamespaceSelector={true}
                multiSelectNamespaces={true}
                highlightedUid={activeMenuId}
                initialSort={{ key: 'updated', direction: 'desc' }}
                resourceType="helmreleases"
                onRefresh={refresh}
                onRowClick={handleRowClick}
                selectable={true}
                selection={selection}
                onBulkDelete={handleBulkDeleteClick}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={handleBulkActionClose} action={bulkActionModal.action} actionLabel="Uninstall" items={bulkActionModal.items} onConfirm={handleBulkActionConfirm} onExportYaml={handleExportYaml} progress={bulkProgress} />

            {upgradeRelease && (
                <HelmUpgradeDialog
                    release={upgradeRelease}
                    onClose={() => setUpgradeRelease(null)}
                    onSuccess={handleUpgradeSuccess}
                />
            )}
        </>
    );
}
