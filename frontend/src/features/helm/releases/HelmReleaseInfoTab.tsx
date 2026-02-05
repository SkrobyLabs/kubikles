import React, { useState } from 'react';
import { CheckIcon, CheckCircleIcon, XCircleIcon, ExclamationTriangleIcon, ArrowPathIcon } from '@heroicons/react/24/outline';
import { formatAge } from '../../../utils/formatting';

// Copyable label component
const CopyableLabel = ({ value, copyValue, className = '' }) => {
    const [copied, setCopied] = useState(false);

    const textToCopy = copyValue || value;

    const handleCopy = async () => {
        if (!textToCopy) return;
        try {
            await navigator.clipboard.writeText(textToCopy);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <button
            onClick={handleCopy}
            className={`inline-flex items-center gap-1 px-2 py-0.5 text-xs rounded border transition-colors cursor-pointer ${
                copied
                    ? 'bg-green-500/20 text-green-400 border-green-500/30'
                    : `bg-gray-500/10 hover:bg-gray-500/20 text-gray-300 border-gray-500/30 ${className}`
            }`}
            title={copied ? 'Copied!' : `Click to copy: ${textToCopy}`}
        >
            {copied ? (
                <>
                    <CheckIcon className="w-3 h-3" />
                    Copied
                </>
            ) : (
                value
            )}
        </button>
    );
};

// Detail row component
const DetailRow = ({ label, value, children }) => (
    <div className="flex py-2 border-b border-border/50">
        <div className="w-32 text-xs font-medium text-gray-500 uppercase tracking-wider shrink-0">
            {label}
        </div>
        <div className="flex-1 text-sm text-gray-200">
            {children || value || <span className="text-gray-500">N/A</span>}
        </div>
    </div>
);

// Status icon helper
const getStatusIcon = (status) => {
    const statusLower = status?.toLowerCase() || '';
    if (statusLower === 'deployed') {
        return <CheckCircleIcon className="h-4 w-4 text-green-400" />;
    } else if (statusLower === 'failed') {
        return <XCircleIcon className="h-4 w-4 text-red-400" />;
    } else if (statusLower.includes('pending')) {
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

export default function HelmReleaseInfoTab({ release }) {
    const formatDate = (timestamp) => {
        if (!timestamp) return 'N/A';
        const date = new Date(timestamp);
        return date.toLocaleString();
    };

    return (
        <div className="flex flex-col h-full overflow-auto p-4">
            <div className="bg-surface rounded-lg border border-border p-4">
                {/* Name */}
                <DetailRow label="Name">
                    <CopyableLabel value={release.name} />
                </DetailRow>

                {/* Namespace */}
                <DetailRow label="Namespace">
                    <CopyableLabel value={release.namespace} />
                </DetailRow>

                {/* Revision */}
                <DetailRow label="Revision" value={release.revision} />

                {/* Status */}
                <DetailRow label="Status">
                    <div className="flex items-center gap-1.5">
                        {getStatusIcon(release.status)}
                        <span className={getStatusClass(release.status)}>{release.status}</span>
                    </div>
                </DetailRow>

                {/* Chart */}
                <DetailRow label="Chart">
                    <CopyableLabel value={release.chart} />
                </DetailRow>

                {/* Chart Version */}
                <DetailRow label="Chart Version">
                    <CopyableLabel value={release.chartVersion} />
                </DetailRow>

                {/* App Version */}
                <DetailRow label="App Version">
                    {release.appVersion ? (
                        <CopyableLabel value={release.appVersion} />
                    ) : (
                        <span className="text-gray-500">N/A</span>
                    )}
                </DetailRow>

                {/* Updated */}
                <DetailRow label="Updated">
                    <div className="flex items-center gap-2">
                        <span>{formatDate(release.updated)}</span>
                        <span className="text-gray-500 text-xs">({formatAge(release.updated)})</span>
                    </div>
                </DetailRow>

                {/* Description */}
                {release.description && (
                    <DetailRow label="Description" value={release.description} />
                )}
            </div>
        </div>
    );
}
