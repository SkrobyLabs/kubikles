import React, { useState, useEffect, useMemo } from 'react';
import { ArrowUturnLeftIcon, CheckCircleIcon, XCircleIcon, ExclamationTriangleIcon } from '@heroicons/react/24/outline';
import { GetHelmReleaseHistory, RollbackHelmRelease } from '../../../../wailsjs/go/main/App';
import { useK8s } from '../../../context/K8sContext';
import { useUI } from '../../../context/UIContext';
import Logger from '../../../utils/Logger';

const formatDate = (timestamp) => {
    if (!timestamp) return '-';
    const date = new Date(timestamp);
    return date.toLocaleString();
};

const getStatusIcon = (status) => {
    const statusLower = status?.toLowerCase() || '';
    if (statusLower === 'deployed') {
        return <CheckCircleIcon className="h-4 w-4 text-green-400" />;
    } else if (statusLower === 'failed') {
        return <XCircleIcon className="h-4 w-4 text-red-400" />;
    } else if (statusLower === 'superseded') {
        return <ExclamationTriangleIcon className="h-4 w-4 text-gray-400" />;
    }
    return null;
};

export default function HelmReleaseHistoryTab({ release, isStale }) {
    const { currentContext, lastRefresh, triggerRefresh } = useK8s();
    const { openModal, closeModal } = useUI();
    const [history, setHistory] = useState([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState(null);

    const currentRevision = release?.revision || 0;

    // Fetch history
    useEffect(() => {
        if (!currentContext || !release || isStale) return;

        const fetchHistory = async () => {
            setLoading(true);
            setError(null);
            try {
                Logger.info("Fetching Helm release history", { namespace: release.namespace, name: release.name });
                const data = await GetHelmReleaseHistory(release.namespace, release.name);
                setHistory(data || []);
            } catch (err) {
                Logger.error("Failed to fetch Helm release history", err);
                setError(err.message || String(err));
            } finally {
                setLoading(false);
            }
        };

        fetchHistory();
    }, [currentContext, release, isStale, lastRefresh]);

    const handleRollback = (revision) => {
        openModal({
            title: `Rollback to Revision ${revision}?`,
            content: `Are you sure you want to rollback "${release.name}" to revision ${revision}? This will create a new revision.`,
            confirmText: 'Rollback',
            confirmStyle: 'primary',
            onConfirm: async () => {
                try {
                    Logger.info("Rolling back Helm release", { namespace: release.namespace, name: release.name, revision });
                    await RollbackHelmRelease(release.namespace, release.name, revision);
                    Logger.info("Rollback successful");
                    closeModal();
                    triggerRefresh();
                } catch (err) {
                    Logger.error("Failed to rollback", err);
                    alert(`Failed to rollback: ${err.message || err}`);
                }
            }
        });
    };

    if (loading && history.length === 0) {
        return (
            <div className="flex items-center justify-center h-full text-gray-400">
                Loading history...
            </div>
        );
    }

    if (error) {
        return (
            <div className="flex items-center justify-center h-full text-red-400">
                {error}
            </div>
        );
    }

    if (history.length === 0) {
        return (
            <div className="flex items-center justify-center h-full text-gray-500">
                No history available
            </div>
        );
    }

    return (
        <div className="flex flex-col h-full overflow-hidden">
            <div className="flex-1 overflow-auto">
                <table className="w-full text-sm">
                    <thead className="sticky top-0 bg-surface border-b border-border">
                        <tr className="text-left text-xs text-gray-500 uppercase tracking-wider">
                            <th className="px-4 py-2 w-24">Revision</th>
                            <th className="px-4 py-2">Status</th>
                            <th className="px-4 py-2">Chart</th>
                            <th className="px-4 py-2">App Version</th>
                            <th className="px-4 py-2">Updated</th>
                            <th className="px-4 py-2">Description</th>
                            <th className="px-4 py-2 w-24"></th>
                        </tr>
                    </thead>
                    <tbody>
                        {history.map((h) => (
                            <tr
                                key={h.revision}
                                className={`border-b border-border/50 hover:bg-white/5 ${h.revision === currentRevision ? 'bg-blue-900/20' : ''}`}
                            >
                                <td className="px-4 py-2">
                                    <span className="font-mono">{h.revision}</span>
                                    {h.revision === currentRevision && (
                                        <span className="ml-2 text-xs text-blue-400">(current)</span>
                                    )}
                                </td>
                                <td className="px-4 py-2">
                                    <div className="flex items-center gap-1.5">
                                        {getStatusIcon(h.status)}
                                        <span>{h.status}</span>
                                    </div>
                                </td>
                                <td className="px-4 py-2 font-mono text-xs text-gray-400">{h.chart}</td>
                                <td className="px-4 py-2 text-gray-400">{h.appVersion || '-'}</td>
                                <td className="px-4 py-2 text-gray-400">{formatDate(h.updated)}</td>
                                <td className="px-4 py-2 text-gray-400 max-w-xs truncate" title={h.description}>
                                    {h.description || '-'}
                                </td>
                                <td className="px-4 py-2">
                                    {h.revision !== currentRevision && h.status?.toLowerCase() !== 'failed' && !isStale && (
                                        <button
                                            onClick={() => handleRollback(h.revision)}
                                            className="flex items-center gap-1 px-2 py-1 text-xs text-blue-400 hover:text-blue-300 hover:bg-blue-900/30 rounded"
                                            title={`Rollback to revision ${h.revision}`}
                                        >
                                            <ArrowUturnLeftIcon className="h-3.5 w-3.5" />
                                            Rollback
                                        </button>
                                    )}
                                </td>
                            </tr>
                        ))}
                    </tbody>
                </table>
            </div>
        </div>
    );
}
