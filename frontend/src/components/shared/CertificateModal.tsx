import React, { useState } from 'react';
import { createPortal } from 'react-dom';
import {
    XMarkIcon,
    ClipboardDocumentIcon,
    CheckIcon,
    ShieldCheckIcon,
    ExclamationTriangleIcon,
    XCircleIcon,
    ClockIcon,
    ChevronLeftIcon,
    ChevronRightIcon
} from '@heroicons/react/24/outline';

// Copy button with feedback
function CopyButton({ value, className = '' }) {
    const [copied, setCopied] = useState(false);

    const handleCopy = async () => {
        try {
            await navigator.clipboard.writeText(value);
            setCopied(true);
            setTimeout(() => setCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy:', err);
        }
    };

    return (
        <button
            onClick={handleCopy}
            className={`p-1 text-gray-500 hover:text-gray-300 transition-colors ${className}`}
            title="Copy to clipboard"
        >
            {copied ? (
                <CheckIcon className="h-4 w-4 text-green-400" />
            ) : (
                <ClipboardDocumentIcon className="h-4 w-4" />
            )}
        </button>
    );
}

// Value row with optional copy button
function ValueRow({ label, value, showCopy = false, mono = false, muted = false }) {
    if (!value) return null;
    return (
        <div className="flex items-start justify-between gap-2 py-1">
            <span className="text-xs text-gray-500 shrink-0">{label}</span>
            <div className="flex items-center gap-1 min-w-0">
                <span className={`text-sm text-right truncate ${mono ? 'font-mono' : ''} ${muted ? 'text-gray-500' : 'text-gray-200'}`}>
                    {value}
                </span>
                {showCopy && <CopyButton value={value} />}
            </div>
        </div>
    );
}

// Badge/chip component
function Badge({ children, className = '' }) {
    return (
        <span className={`inline-flex items-center px-2 py-0.5 text-xs rounded border ${className}`}>
            {children}
        </span>
    );
}

// SAN item with type badge
function SANItem({ type, value }) {
    const typeColors = {
        DNS: 'bg-blue-900/30 text-blue-400 border-blue-500/30',
        IP: 'bg-purple-900/30 text-purple-400 border-purple-500/30',
        Email: 'bg-amber-900/30 text-amber-400 border-amber-500/30',
    };

    return (
        <div className="flex items-center justify-between gap-2 py-1.5 px-2 bg-background rounded">
            <div className="flex items-center gap-2 min-w-0">
                <Badge className={typeColors[type] || 'bg-gray-800 text-gray-400 border-gray-600'}>
                    {type}
                </Badge>
                <span className="text-sm font-mono text-gray-200 truncate">{value}</span>
            </div>
            <CopyButton value={value} />
        </div>
    );
}

export default function CertificateModal({ certificates, pemData, onClose }) {
    const [showRaw, setShowRaw] = useState(false);
    const [pemCopied, setPemCopied] = useState(false);
    const [currentIndex, setCurrentIndex] = useState(0);

    // Handle both single cert (legacy) and array of certs
    const certArray = Array.isArray(certificates) ? certificates : (certificates ? [certificates] : []);

    if (certArray.length === 0 || !certArray[0]?.isCertificate) return null;

    const certInfo = certArray[currentIndex];
    const totalCerts = certArray.length;
    const hasMultiple = totalCerts > 1;

    // Get cert type label for navigation
    const getCertTypeLabel = (cert, index) => {
        if (index === 0) return 'Leaf';
        if (index === totalCerts - 1 && totalCerts > 1) return 'Root';
        return 'Intermediate';
    };

    // Determine status
    const getStatus = () => {
        if (certInfo.isExpired) {
            return {
                label: 'EXPIRED',
                icon: XCircleIcon,
                color: 'bg-red-900/50 border-red-500/50 text-red-400',
                barColor: 'bg-red-500'
            };
        }
        if (certInfo.isNotYetValid) {
            return {
                label: 'NOT YET VALID',
                icon: ClockIcon,
                color: 'bg-gray-800/50 border-gray-500/50 text-gray-400',
                barColor: 'bg-gray-500'
            };
        }
        if (certInfo.daysUntilExpiry <= 7) {
            return {
                label: 'CRITICAL',
                icon: ExclamationTriangleIcon,
                color: 'bg-red-900/50 border-red-500/50 text-red-400',
                barColor: 'bg-red-500'
            };
        }
        if (certInfo.daysUntilExpiry <= 30) {
            return {
                label: 'EXPIRING SOON',
                icon: ExclamationTriangleIcon,
                color: 'bg-amber-900/50 border-amber-500/50 text-amber-400',
                barColor: 'bg-amber-500'
            };
        }
        return {
            label: 'VALID',
            icon: ShieldCheckIcon,
            color: 'bg-emerald-900/50 border-emerald-500/50 text-emerald-400',
            barColor: 'bg-emerald-500'
        };
    };

    const status = getStatus();
    const StatusIcon = status.icon;

    // Format expiry text
    const getExpiryText = () => {
        if (certInfo.isExpired) {
            return `Expired ${Math.abs(certInfo.daysUntilExpiry)} days ago`;
        }
        if (certInfo.isNotYetValid) {
            return 'Not yet valid';
        }
        if (certInfo.daysUntilExpiry === 0) return 'Expires today';
        if (certInfo.daysUntilExpiry === 1) return 'Expires tomorrow';
        return `Expires in ${certInfo.daysUntilExpiry} days`;
    };

    // Format dates
    const formatDate = (isoDate) => {
        try {
            const date = new Date(isoDate);
            return date.toLocaleDateString('en-US', {
                month: 'short',
                day: 'numeric',
                year: 'numeric',
                hour: '2-digit',
                minute: '2-digit',
                timeZoneName: 'short'
            });
        } catch {
            return isoDate;
        }
    };

    // Copy PEM handler
    const handleCopyPem = async () => {
        try {
            await navigator.clipboard.writeText(pemData);
            setPemCopied(true);
            setTimeout(() => setPemCopied(false), 2000);
        } catch (err) {
            console.error('Failed to copy PEM:', err);
        }
    };

    // Collect all SANs
    const allSANs = [
        ...(certInfo.dnsNames || []).map(v => ({ type: 'DNS', value: v })),
        ...(certInfo.ipAddresses || []).map(v => ({ type: 'IP', value: v })),
        ...(certInfo.emailAddresses || []).map(v => ({ type: 'Email', value: v })),
    ];

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center p-4">
            <div className="absolute inset-0 bg-black/60" onClick={onClose} />
            <div className="relative bg-surface border border-border rounded-lg shadow-xl w-full max-w-2xl max-h-[90vh] flex flex-col">
                {/* Header */}
                <div className="flex items-center justify-between px-4 py-3 border-b border-border shrink-0">
                    <div className="flex items-center gap-3">
                        <h2 className="text-lg font-medium text-white">Certificate Details</h2>
                        {hasMultiple && (
                            <div className="flex items-center gap-1">
                                <button
                                    onClick={() => setCurrentIndex(i => Math.max(0, i - 1))}
                                    disabled={currentIndex === 0}
                                    className="p-1 text-gray-400 hover:text-white disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                                    title="Previous certificate"
                                >
                                    <ChevronLeftIcon className="h-4 w-4" />
                                </button>
                                <span className="text-sm text-gray-400 min-w-[80px] text-center">
                                    {currentIndex + 1} of {totalCerts}
                                </span>
                                <button
                                    onClick={() => setCurrentIndex(i => Math.min(totalCerts - 1, i + 1))}
                                    disabled={currentIndex === totalCerts - 1}
                                    className="p-1 text-gray-400 hover:text-white disabled:opacity-30 disabled:cursor-not-allowed transition-colors"
                                    title="Next certificate"
                                >
                                    <ChevronRightIcon className="h-4 w-4" />
                                </button>
                                <span className="text-xs px-2 py-0.5 rounded bg-surface-light text-gray-400 ml-1">
                                    {getCertTypeLabel(certInfo, currentIndex)}
                                </span>
                            </div>
                        )}
                    </div>
                    <button
                        onClick={onClose}
                        className="p-1 text-gray-400 hover:text-white transition-colors"
                    >
                        <XMarkIcon className="h-5 w-5" />
                    </button>
                </div>

                {/* Content - Scrollable */}
                <div className="flex-1 overflow-y-auto p-4 space-y-4">
                    {showRaw ? (
                        /* Raw PEM view */
                        <div className="bg-background rounded-lg p-4">
                            <pre className="text-xs font-mono text-gray-300 whitespace-pre-wrap break-all">
                                {pemData}
                            </pre>
                        </div>
                    ) : (
                        <>
                            {/* Status Banner */}
                            <div className={`rounded-lg border p-4 ${status.color}`}>
                                <div className="flex items-center justify-between mb-2">
                                    <div className="flex items-center gap-2">
                                        <StatusIcon className="h-5 w-5" />
                                        <span className="font-medium">{status.label}</span>
                                    </div>
                                    <span className="text-sm">{getExpiryText()}</span>
                                </div>
                                {/* Progress bar */}
                                <div className="relative h-2 bg-black/30 rounded-full overflow-hidden">
                                    <div
                                        className={`absolute inset-y-0 left-0 ${status.barColor} transition-all`}
                                        style={{ width: `${Math.min(100, Math.max(0, certInfo.validityPercentage))}%` }}
                                    />
                                </div>
                                <div className="flex justify-between mt-1 text-xs opacity-75">
                                    <span>{formatDate(certInfo.notBefore).split(',')[0]}</span>
                                    <span>{certInfo.validityPercentage}% elapsed</span>
                                    <span>{formatDate(certInfo.notAfter).split(',')[0]}</span>
                                </div>
                            </div>

                            {/* Subject / Issuer Grid */}
                            <div className="grid grid-cols-2 gap-4">
                                {/* Subject */}
                                <div className="bg-surface-light rounded-lg p-3">
                                    <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Subject</h3>
                                    <div className="flex items-center gap-1 mb-1">
                                        <span className="text-sm font-medium text-gray-200 truncate" title={certInfo.subject.commonName}>
                                            {certInfo.subject.commonName || '(none)'}
                                        </span>
                                        {certInfo.subject.commonName && <CopyButton value={certInfo.subject.commonName} />}
                                    </div>
                                    {certInfo.subject.organization && (
                                        <div className="text-xs text-gray-500">{certInfo.subject.organization}</div>
                                    )}
                                    {certInfo.subject.organizationalUnit && (
                                        <div className="text-xs text-gray-500">{certInfo.subject.organizationalUnit}</div>
                                    )}
                                </div>

                                {/* Issuer */}
                                <div className="bg-surface-light rounded-lg p-3">
                                    <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Issuer</h3>
                                    <div className="flex items-center gap-1 mb-1">
                                        <span className="text-sm font-medium text-gray-200 truncate" title={certInfo.issuer.commonName}>
                                            {certInfo.issuer.commonName || '(none)'}
                                        </span>
                                        {certInfo.issuer.commonName && <CopyButton value={certInfo.issuer.commonName} />}
                                    </div>
                                    {certInfo.issuer.organization && (
                                        <div className="text-xs text-gray-500">{certInfo.issuer.organization}</div>
                                    )}
                                    {certInfo.issuer.country && (
                                        <div className="text-xs text-gray-500">{certInfo.issuer.country}</div>
                                    )}
                                </div>
                            </div>

                            {/* SANs */}
                            {allSANs.length > 0 && (
                                <div className="bg-surface-light rounded-lg p-3">
                                    <div className="flex items-center justify-between mb-2">
                                        <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide">
                                            Subject Alternative Names
                                        </h3>
                                        <span className="text-xs text-gray-500">{allSANs.length} {allSANs.length === 1 ? 'name' : 'names'}</span>
                                    </div>
                                    <div className="space-y-1 max-h-32 overflow-y-auto">
                                        {allSANs.map((san, i) => (
                                            <SANItem key={i} type={san.type} value={san.value} />
                                        ))}
                                    </div>
                                </div>
                            )}

                            {/* Key Info / Usage Grid */}
                            <div className="grid grid-cols-2 gap-4">
                                {/* Key Info */}
                                <div className="bg-surface-light rounded-lg p-3">
                                    <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Key Info</h3>
                                    <div className="space-y-0.5">
                                        <ValueRow
                                            label="Algorithm"
                                            value={certInfo.publicKey?.algorithm}
                                        />
                                        <ValueRow
                                            label="Size"
                                            value={certInfo.publicKey?.size ? `${certInfo.publicKey.size} bits` : null}
                                        />
                                        <ValueRow
                                            label="Signature"
                                            value={certInfo.signatureAlgorithm}
                                        />
                                    </div>
                                </div>

                                {/* Key Usage */}
                                <div className="bg-surface-light rounded-lg p-3">
                                    <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Key Usage</h3>
                                    <div className="flex flex-wrap gap-1">
                                        {(certInfo.keyUsage || []).map((usage, i) => (
                                            <Badge key={i} className="bg-zinc-800 text-zinc-300 border-zinc-600">
                                                {usage}
                                            </Badge>
                                        ))}
                                        {(certInfo.extKeyUsage || []).map((usage, i) => (
                                            <Badge key={`ext-${i}`} className="bg-blue-900/30 text-blue-400 border-blue-500/30">
                                                {usage.replace(' Authentication', ' Auth')}
                                            </Badge>
                                        ))}
                                        {(!certInfo.keyUsage?.length && !certInfo.extKeyUsage?.length) && (
                                            <span className="text-xs text-gray-500">None specified</span>
                                        )}
                                    </div>
                                </div>
                            </div>

                            {/* Fingerprints */}
                            <div className="bg-surface-light rounded-lg p-3">
                                <h3 className="text-xs font-medium text-gray-400 uppercase tracking-wide mb-2">Fingerprints</h3>
                                <div className="space-y-2">
                                    <div className="flex items-start justify-between gap-2">
                                        <span className="text-xs text-gray-500 shrink-0">SHA-256</span>
                                        <div className="flex items-center gap-1 min-w-0">
                                            <span className="text-xs font-mono text-gray-300 truncate" title={certInfo.fingerprintSHA256}>
                                                {certInfo.fingerprintSHA256}
                                            </span>
                                            <CopyButton value={certInfo.fingerprintSHA256} />
                                        </div>
                                    </div>
                                    <div className="flex items-start justify-between gap-2">
                                        <span className="text-xs text-gray-500 shrink-0">SHA-1</span>
                                        <div className="flex items-center gap-1 min-w-0">
                                            <span className="text-xs font-mono text-gray-500 truncate" title={certInfo.fingerprintSHA1}>
                                                {certInfo.fingerprintSHA1}
                                            </span>
                                            <CopyButton value={certInfo.fingerprintSHA1} className="opacity-50" />
                                        </div>
                                    </div>
                                    <div className="flex items-start justify-between gap-2">
                                        <span className="text-xs text-gray-500 shrink-0">Serial</span>
                                        <div className="flex items-center gap-1 min-w-0">
                                            <span className="text-xs font-mono text-gray-300 truncate" title={certInfo.serialNumber}>
                                                {certInfo.serialNumber}
                                            </span>
                                            <CopyButton value={certInfo.serialNumber} />
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </>
                    )}
                </div>

                {/* Footer */}
                <div className="flex items-center justify-between px-4 py-3 border-t border-border shrink-0">
                    <div className="flex items-center gap-2">
                        <button
                            onClick={handleCopyPem}
                            className="px-3 py-1.5 text-sm text-gray-400 hover:text-white hover:bg-surface-light rounded transition-colors"
                        >
                            {pemCopied ? 'Copied!' : 'Copy PEM'}
                        </button>
                        <button
                            onClick={() => setShowRaw(!showRaw)}
                            className={`px-3 py-1.5 text-sm rounded transition-colors ${
                                showRaw
                                    ? 'bg-primary text-white'
                                    : 'text-gray-400 hover:text-white hover:bg-surface-light'
                            }`}
                        >
                            {showRaw ? 'Show Details' : 'View Raw'}
                        </button>
                    </div>
                    <button
                        onClick={onClose}
                        className="px-4 py-1.5 text-sm bg-surface-light hover:bg-surface-hover text-gray-300 rounded transition-colors"
                    >
                        Close
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
}
