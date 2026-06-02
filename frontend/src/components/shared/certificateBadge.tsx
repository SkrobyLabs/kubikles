import React from 'react';
import { ShieldCheckIcon } from '@heroicons/react/24/outline';

export const makeCertificateCacheEntry = (certificates: any[] = []) => ({
    certificates,
    firstCert: certificates[0] || null,
});

const certBucketCounts = (certificates: any[]) => certificates.reduce((counts, cert) => {
    if (cert.isExpired || cert.daysUntilExpiry <= 7) {
        counts.expired += 1;
    } else if (cert.daysUntilExpiry <= 30) {
        counts.warning += 1;
    } else {
        counts.healthy += 1;
    }
    return counts;
}, { healthy: 0, warning: 0, expired: 0 });

const bucketStyle = (colorVar: string) => ({
    color: `var(${colorVar})`,
    borderColor: `color-mix(in srgb, var(${colorVar}) 35%, transparent)`,
    backgroundColor: `color-mix(in srgb, var(${colorVar}) 16%, transparent)`,
});

export const getExpiryText = (certInfo: any) => {
    if (!certInfo) return '';
    if (certInfo.isExpired) return 'Expired';
    if (certInfo.daysUntilExpiry === 0) return 'Expires today';
    if (certInfo.daysUntilExpiry === 1) return 'Expires tomorrow';
    return `${certInfo.daysUntilExpiry}d`;
};

export const getCertSummary = (certInfo: any) => {
    if (!certInfo) return '';
    if (certInfo.dnsNames?.length > 0) {
        const first = certInfo.dnsNames[0];
        if (certInfo.dnsNames.length > 1) {
            return `${first} +${certInfo.dnsNames.length - 1}`;
        }
        return first;
    }
    return certInfo.subject?.commonName || '';
};

export const getCertBadgeStyle = (certInfo: any) => {
    if (!certInfo) return bucketStyle('--color-text-muted');
    if (certInfo.isExpired || certInfo.daysUntilExpiry <= 7) {
        return bucketStyle('--color-error');
    }
    if (certInfo.daysUntilExpiry <= 30) {
        return bucketStyle('--color-warning');
    }
    return bucketStyle('--color-success');
};

export function CertificateBadge({
    cacheEntry,
    onClick,
    disabled = false,
}: {
    cacheEntry: any;
    onClick: () => void;
    disabled?: boolean;
}) {
    const certificates = cacheEntry?.certificates || [];
    const firstCert = cacheEntry?.firstCert;
    if (!firstCert) return null;

    const counts = certBucketCounts(certificates);
    const isChain = certificates.length > 1;

    return (
        <button
            type="button"
            onClick={onClick}
            disabled={disabled}
            title="View certificate details"
            aria-label="View certificate details"
            className="inline-flex items-center gap-1.5 px-2 py-0.5 text-xs rounded border transition-colors hover:bg-white/5 disabled:opacity-60 disabled:cursor-not-allowed"
            style={isChain ? undefined : getCertBadgeStyle(firstCert)}
        >
            <ShieldCheckIcon className="h-3 w-3" />
            {isChain ? (
                <span className="inline-flex items-center gap-1">
                    {counts.healthy > 0 && (
                        <span className="px-1.5 py-0.5 rounded border" style={bucketStyle('--color-success')}>
                            {counts.healthy} &gt;30d
                        </span>
                    )}
                    {counts.warning > 0 && (
                        <span className="px-1.5 py-0.5 rounded border" style={bucketStyle('--color-warning')}>
                            {counts.warning} &gt;7d
                        </span>
                    )}
                    {(counts.expired > 0 || (counts.healthy === 0 && counts.warning === 0)) && (
                        <span className="px-1.5 py-0.5 rounded border" style={bucketStyle('--color-error')}>
                            {counts.expired} &lt;=7d/expired
                        </span>
                    )}
                </span>
            ) : (
                <>
                    <span>{getExpiryText(firstCert)}</span>
                    {getCertSummary(firstCert) && (
                        <span style={{ color: 'var(--color-text-muted)' }}>({getCertSummary(firstCert)})</span>
                    )}
                </>
            )}
        </button>
    );
}
