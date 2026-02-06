import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, ArrowPathIcon, CheckCircleIcon, ExclamationTriangleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '~/components/shared/ResourceList';
import BulkActionModal from '~/components/shared/BulkActionModal';
import HelmReleaseActionsMenu from './HelmReleaseActionsMenu';
import HelmUpgradeDialog from './HelmUpgradeDialog';
import { useHelmReleases } from '~/hooks/useHelmReleases';
import { useHelmReleaseActions } from './useHelmReleaseActions';
import { useK8s } from '~/context';
import { useUI } from '~/context';
import { useNotification } from '~/context';
import { useSelection } from '~/hooks/useSelection';
import { useBulkActions } from '~/hooks/useBulkActions';
import { ForceHelmReleaseStatus, UninstallHelmRelease, GetHelmReleaseValues } from 'wailsjs/go/main/App';
import { useMenuPosition } from '~/hooks/useMenuPosition';

const formatAge = (timestamp: any) => {
    if (!timestamp) return '-';
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now.getTime() - date.getTime();

    const seconds = Math.floor(diff / 1000);
    if (seconds < 60) return `${seconds}s`;

    const minutes = Math.floor(seconds / 60);
    if (minutes < 60) return `${minutes}m`;

    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h`;

    const days = Math.floor(hours / 24);
    return `${days}d`;
};

const getStatusIcon = (status: any) => {
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

const getStatusClass = (status: any) => {
    const statusLower = status?.toLowerCase() || '';
    if (statusLower === 'deployed') return 'text-green-400';
    if (statusLower === 'failed') return 'text-red-400';
    if (statusLower.includes('pending')) return 'text-yellow-400';
    return 'text-gray-400';
};

export default function HelmReleaseList({ isVisible }: { isVisible: boolean }) {
    const { currentContext, selectedNamespaces, setSelectedNamespaces, namespaces } = useK8s();
    const { openModal, closeModal } = useUI();
    const { activeMenuId, menuPosition, handleMenuOpenChange } = useMenuPosition();
    const { addNotification } = useNotification();
    const [upgradeRelease, setUpgradeRelease] = useState<any>(null);
    const selection = useSelection();

    const { releases, loading, refresh } = useHelmReleases(currentContext, selectedNamespaces, isVisible) as any;
    const {
        handleOpenDetails,
        handleViewValues,
        handleViewHistory,
        handleRollback,
        handleUninstall
    } = useHelmReleaseActions();

    // Wrapper for UninstallHelmRelease to match useBulkActions API signature (context, namespace, name)
    const uninstallApi = useCallback(async (_context: any, namespace: any, name: any) => {
        return UninstallHelmRelease(namespace, name);
    }, []);

    // Custom export for Helm releases - includes chart info in YAML header
    const handleExportYaml = useCallback(async (items: any[], { onProgress, signal }: any = {}) => {
        const { SaveYamlBackup } = await import('../../../../wailsjs/go/main/App');
        const entries = [];
        for (let i = 0; i < items.length; i++) {
            if (signal?.aborted) break;
            const item = items[i];
            const namespace = item.metadata?.namespace || item.namespace;
            const name = item.metadata?.name || item.name;
            try {
                const values = await GetHelmReleaseValues(namespace, name);
                entries.push({ namespace, name, kind: 'HelmRelease', yaml: `# Helm Release: ${name}\n# Chart: ${item.chart}-${item.chartVersion}\n# Values:\n${values}` });
            } catch (err: any) {
                entries.push({ namespace, name, kind: 'HelmRelease', yaml: `# Failed: ${err}` });
            }
            onProgress?.(i + 1, items.length);
        }
        if (entries.length === 0) return;
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19);
        try { await SaveYamlBackup(entries, `helmreleases-backup-${timestamp}.zip`); } catch (err: any) { if (err?.toString()) addNotification({ type: 'error', title: 'Failed to save backup', message: String(err) }); }
    }, []);

    const {
        bulkActionModal,
        bulkProgress,
        openBulkDelete,
        closeBulkAction: closeBulkActionBase,
        confirmBulkAction,
    } = useBulkActions({
        resourceLabel: 'Helm Release',
        resourceType: 'helmreleases',
        isNamespaced: true,
        deleteApi: uninstallApi as any,
        getYamlApi: GetHelmReleaseValues,

    });

    // Wrap closeBulkAction to also refresh the list
    const closeBulkAction = useCallback(() => {
        closeBulkActionBase();
        refresh();
    }, [closeBulkActionBase, refresh]);

    const handleUpgrade = useCallback((release: any) => {
        setUpgradeRelease(release);
    }, []);

    const handleForceStatus = useCallback((release: any) => {
        openModal({
            title: `Force Status: ${release.name}`,
            content: `Force release "${release.name}" status to "deployed"? This will mark the release as successfully deployed without making any changes to the actual resources.`,
            confirmText: 'Force Deployed',
            confirmStyle: 'primary',
            onConfirm: () => {
                // Close modal immediately - operation runs in background
                closeModal();

                // Show in-progress notification
                addNotification({
                    type: 'info',
                    title: 'Updating status',
                    message: `Forcing "${release.name}" status to deployed...`,
                    duration: 3000
                });

                // Run operation asynchronously without blocking
                ForceHelmReleaseStatus(release.namespace, release.name, 'deployed')
                    .then(() => {
                        addNotification({
                            type: 'success',
                            title: 'Status updated',
                            message: `Release "${release.name}" marked as deployed`
                        });
                        refresh();
                    })
                    .catch((err: any) => {
                        addNotification({
                            type: 'error',
                            title: 'Failed to update status',
                            message: err?.message || String(err)
                        });
                    });
            }
        });
    }, [openModal, closeModal, addNotification, refresh]);

    const handleUpgradeSuccess = useCallback(() => {
        setUpgradeRelease(null);
        refresh();
    }, [refresh]);

    const handleRowClick = useCallback((item: any) => {
        handleOpenDetails(item);
    }, [handleOpenDetails]);

    const columns = useMemo(() => [
        {
            key: 'name',
            label: 'Name',
            render: (item: any) => item.name,
            getValue: (item: any) => item.name,
            initialSort: 'asc'
        },
        {
            key: 'namespace',
            label: 'Namespace',
            render: (item: any) => item.namespace,
            getValue: (item: any) => item.namespace
        },
        {
            key: 'revision',
            label: 'Rev',
            render: (item: any) => item.revision,
            getValue: (item: any) => item.revision
        },
        {
            key: 'status',
            label: 'Status',
            render: (item: any) => (
                <div className="flex items-center gap-1.5">
                    {getStatusIcon(item.status)}
                    <span className={getStatusClass(item.status)}>{item.status}</span>
                </div>
            ),
            getValue: (item: any) => item.status
        },
        {
            key: 'chart',
            label: 'Chart',
            render: (item: any) => (
                <span className="font-mono text-xs">
                    {item.chart}
                    {item.chartVersion && <span className="text-gray-500">-{item.chartVersion}</span>}
                </span>
            ),
            getValue: (item: any) => `${item.chart}-${item.chartVersion || ''}`
        },
        {
            key: 'appVersion',
            label: 'App Version',
            render: (item: any) => item.appVersion || '-',
            getValue: (item: any) => item.appVersion || ''
        },
        {
            key: 'updated',
            label: 'Updated',
            render: (item: any) => formatAge(item.updated),
            getValue: (item: any) => item.updated
        },
        {
            key: 'actions',
            label: <EllipsisVerticalIcon className="h-5 w-5" />,
            align: 'center',
            render: (item: any) => (
                <HelmReleaseActionsMenu
                    release={item}
                    isOpen={activeMenuId === `helm-${item.namespace}-${item.name}`}
                    menuPosition={menuPosition}
                    onOpenChange={(isOpen: any, buttonElement: any) => handleMenuOpenChange(isOpen, `helm-${item.namespace}-${item.name}`, buttonElement)}
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
        return releases.map((r: any) => ({
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
                onBulkDelete={openBulkDelete}
            />
            <BulkActionModal isOpen={bulkActionModal.isOpen} onClose={closeBulkAction} action={bulkActionModal.action || ''} actionLabel="Uninstall" items={bulkActionModal.items} onConfirm={confirmBulkAction} onExportYaml={handleExportYaml} progress={bulkProgress} />

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
