import React, { useMemo, useState, useCallback } from 'react';
import { EllipsisVerticalIcon, ArrowPathIcon, CheckCircleIcon, ExclamationTriangleIcon, XCircleIcon } from '@heroicons/react/24/outline';
import ResourceList from '../../../components/shared/ResourceList';
import HelmReleaseActionsMenu from './HelmReleaseActionsMenu';
import { useHelmReleases } from '../../../hooks/useHelmReleases';
import { useHelmReleaseActions } from './useHelmReleaseActions';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';

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
    const { activeMenuId, setActiveMenuId } = useUI();
    const [menuPosition, setMenuPosition] = useState({ top: 0, left: 0 });

    const { releases, loading, refresh } = useHelmReleases(currentContext, selectedNamespaces, isVisible);
    const {
        handleOpenDetails,
        handleViewValues,
        handleViewHistory,
        handleRollback,
        handleUninstall
    } = useHelmReleaseActions();

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
                    onUninstall={() => handleUninstall(item)}
                />
            ),
            isColumnSelector: true,
            disableSort: true
        },
    ], [activeMenuId, menuPosition, handleMenuOpenChange, handleOpenDetails, handleViewValues, handleViewHistory, handleRollback, handleUninstall]);

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
        />
    );
}
