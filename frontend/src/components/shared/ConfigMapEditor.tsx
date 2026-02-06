import React, { useState, useEffect, useRef } from 'react';
import Editor from '@monaco-editor/react';
import { GetConfigMapYaml, UpdateConfigMapYaml, GetConfigMapData, UpdateConfigMapData, GetCertificateInfo, GetAllCertificateInfo } from 'wailsjs/go/main/App';
import { useK8s } from '~/context';
import { useNotification } from '~/context';
import Logger from '~/utils/Logger';
import { TrashIcon, PlusIcon, LockClosedIcon, MagnifyingGlassIcon, ShieldCheckIcon } from '@heroicons/react/24/outline';
import CertificateModal from './CertificateModal';

const MODE_YAML = 'yaml';
const MODE_KEYVALUE = 'keyvalue';

// Convert object to array with stable IDs
const objectToEntries = (obj: any) => {
    return Object.entries(obj || {}).map(([key, value], index) => ({
        id: `entry-${Date.now()}-${index}`,
        key,
        value
    }));
};

// Convert array back to object for saving
const entriesToObject = (entries: any) => {
    const result: Record<string, any> = {};
    entries.forEach(({ key, value }: { key: any; value: any }) => {
        if (key) result[key] = value;
    });
    return result;
};

export default function ConfigMapEditor({ namespace, resourceName, onClose, tabContext = '', initialMode = MODE_YAML }: any) {
    const { currentContext } = useK8s();
    const { addNotification } = useNotification();
    const [mode, setMode] = useState(initialMode);
    const [yamlContent, setYamlContent] = useState('');
    const [configMapEntries, setConfigMapEntries] = useState<any[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<any>(null);
    const [saving, setSaving] = useState(false);
    const [keySearchTerm, setKeySearchTerm] = useState('');
    const editorRef = useRef<any>(null);
    const nextIdRef = useRef(0);
    const [certInfoCache, setCertInfoCache] = useState<Record<string, any>>({});
    const [selectedCert, setSelectedCert] = useState<any>(null); // { certificates: [], pemData } for modal

    // Check if this tab is stale (opened in a different context)
    const isStale = tabContext && tabContext !== currentContext;

    // Check if a value looks like a PEM certificate
    const isPEMCertificate = (value: any) => value?.includes('-----BEGIN CERTIFICATE-----');

    // Load certificate info for entries that look like certificates
    useEffect(() => {
        const loadCertInfo = async () => {
            for (const entry of configMapEntries) {
                if (isPEMCertificate(entry.value) && !certInfoCache[entry.id]) {
                    try {
                        const info = await GetCertificateInfo(entry.value);
                        if (info?.isCertificate) {
                            setCertInfoCache(prev => ({ ...prev, [entry.id]: info }));
                        }
                    } catch (err: any) {
                        Logger.debug("Failed to parse certificate", { key: entry.key, error: err });
                    }
                }
            }
        };
        if (mode === MODE_KEYVALUE && configMapEntries.length > 0) {
            loadCertInfo();
        }
    }, [configMapEntries, mode]);

    // Handle opening certificate modal
    const handleViewCertificate = async (entry: any) => {
        try {
            const certificates = await GetAllCertificateInfo(entry.value);
            if (certificates?.length > 0) {
                if (!certInfoCache[entry.id]) {
                    setCertInfoCache(prev => ({ ...prev, [entry.id]: certificates[0] }));
                }
                setSelectedCert({ certificates, pemData: entry.value });
            }
        } catch (err: any) {
            Logger.error("Failed to get certificate info", err);
            addNotification({ type: 'error', title: 'Failed to parse certificate', message: String(err) });
        }
    };

    // Get expiry badge color based on days until expiry
    const getExpiryBadgeStyle = (certInfo: any) => {
        if (!certInfo) return null;
        if (certInfo.isExpired || certInfo.daysUntilExpiry < 7) {
            return 'bg-red-600/20 text-red-400 border-red-500/30';
        }
        if (certInfo.daysUntilExpiry <= 30) {
            return 'bg-amber-600/20 text-amber-400 border-amber-500/30';
        }
        return 'bg-green-600/20 text-green-400 border-green-500/30';
    };

    // Format expiry text
    const getExpiryText = (certInfo: any) => {
        if (!certInfo) return '';
        if (certInfo.isExpired) return 'Expired';
        if (certInfo.daysUntilExpiry === 0) return 'Expires today';
        if (certInfo.daysUntilExpiry === 1) return 'Expires tomorrow';
        return `${certInfo.daysUntilExpiry}d`;
    };

    // Get first SAN or subject for display
    const getCertSummary = (certInfo: any) => {
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

    const generateId = () => {
        nextIdRef.current += 1;
        return `entry-${nextIdRef.current}`;
    };

    useEffect(() => {
        fetchData();
    }, [namespace, resourceName]);

    const fetchData = async () => {
        setLoading(true);
        setError(null);
        Logger.debug("Fetching configmap data...", { namespace, name: resourceName });
        try {
            const [yaml, data] = await Promise.all([
                GetConfigMapYaml(namespace, resourceName),
                GetConfigMapData(namespace, resourceName)
            ]);
            setYamlContent(yaml);
            setConfigMapEntries(objectToEntries(data));
            Logger.info("ConfigMap data fetched successfully", { namespace, name: resourceName });
        } catch (err: any) {
            Logger.error("Failed to load configmap", err);
            setError(`Failed to load configmap: ${err}`);
        } finally {
            setLoading(false);
        }
    };

    const handleSaveYaml = async () => {
        setSaving(true);
        Logger.info("Saving YAML...", { namespace, name: resourceName });
        try {
            await UpdateConfigMapYaml(namespace, resourceName, yamlContent);
            Logger.info("YAML saved successfully", { namespace, name: resourceName });
            addNotification({ type: 'success', title: 'ConfigMap saved successfully', message: '' });
            // Refresh key-value data after YAML save
            const data = await GetConfigMapData(namespace, resourceName);
            setConfigMapEntries(objectToEntries(data));
        } catch (err: any) {
            Logger.error("Failed to save configmap", err);
            addNotification({ type: 'error', title: 'Failed to save ConfigMap', message: String(err) });
        } finally {
            setSaving(false);
        }
    };

    const handleSaveKeyValue = async () => {
        setSaving(true);
        Logger.info("Saving configmap data...", { namespace, name: resourceName });
        try {
            const dataToSave = entriesToObject(configMapEntries);
            await UpdateConfigMapData(namespace, resourceName, dataToSave);
            Logger.info("ConfigMap data saved successfully", { namespace, name: resourceName });
            addNotification({ type: 'success', title: 'ConfigMap saved successfully', message: '' });
            // Refresh YAML after key-value save
            const yaml = await GetConfigMapYaml(namespace, resourceName);
            setYamlContent(yaml);
        } catch (err: any) {
            Logger.error("Failed to save configmap", err);
            addNotification({ type: 'error', title: 'Failed to save ConfigMap', message: String(err) });
        } finally {
            setSaving(false);
        }
    };

    const handleSave = () => {
        if (mode === MODE_YAML) {
            handleSaveYaml();
        } else {
            handleSaveKeyValue();
        }
    };

    const handleEditorDidMount = (editor: any, monaco: any) => {
        editorRef.current = editor;
        editor.updateOptions({
            minimap: { enabled: true },
            scrollBeyondLastLine: false,
            fontSize: 13,
            lineNumbers: 'on',
            folding: true,
            renderWhitespace: 'selection',
            wordWrap: 'off',
        });
        editor.addCommand(monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS, () => {
            if (!isStale) {
                handleSave();
            }
        });
    };

    const handleKeyChange = (id: any, newKey: any) => {
        setConfigMapEntries(entries =>
            entries.map((entry: any) =>
                entry.id === id ? { ...entry, key: newKey } : entry
            )
        );
    };

    const handleValueChange = (id: any, newValue: any) => {
        setConfigMapEntries(entries =>
            entries.map((entry: any) =>
                entry.id === id ? { ...entry, value: newValue } : entry
            )
        );
    };

    const handleDeleteEntry = (id: any) => {
        setConfigMapEntries(entries => entries.filter((entry: any) => entry.id !== id));
    };

    const handleAddKey = () => {
        const existingKeys = new Set(configMapEntries.map((e: any) => e.key));
        let newKey = 'NEW_KEY';
        let counter = 1;
        while (existingKeys.has(newKey)) {
            newKey = `NEW_KEY_${counter}`;
            counter++;
        }
        setConfigMapEntries([...configMapEntries, { id: generateId(), key: newKey, value: '' }]);
    };

    if (loading) {
        return (
            <div className="flex items-center justify-center h-full text-gray-400">
                <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary mr-2"></div>
                Loading configmap...
            </div>
        );
    }

    if (error) {
        return (
            <div className="p-4 text-red-400">
                {error}
            </div>
        );
    }

    return (
        <div className="flex flex-col h-full bg-background">
            {/* Stale Tab Banner */}
            {isStale && (
                <div className="flex items-center gap-2 px-4 py-2 bg-amber-900/30 border-b border-amber-500/50 text-amber-400 shrink-0">
                    <LockClosedIcon className="h-5 w-5" />
                    <span className="text-sm">
                        Read-only: This configmap is from context <span className="font-medium">{tabContext}</span>. Switch back to edit.
                    </span>
                </div>
            )}

            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {namespace}/{resourceName}
                    </div>
                    {/* Mode Toggle */}
                    <div className="flex items-center bg-surface-light rounded-md p-0.5">
                        <button
                            onClick={() => setMode(MODE_YAML)}
                            className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                mode === MODE_YAML
                                    ? 'bg-primary text-white'
                                    : 'text-gray-400 hover:text-white'
                            }`}
                        >
                            YAML
                        </button>
                        <button
                            onClick={() => setMode(MODE_KEYVALUE)}
                            className={`px-3 py-1 text-xs font-medium rounded transition-colors ${
                                mode === MODE_KEYVALUE
                                    ? 'bg-primary text-white'
                                    : 'text-gray-400 hover:text-white'
                            }`}
                        >
                            Key-Value
                        </button>
                    </div>
                    {/* Key Search - only visible in Key-Value mode */}
                    {mode === MODE_KEYVALUE && (
                        <div className="relative w-48">
                            <MagnifyingGlassIcon className="h-4 w-4 text-gray-400 absolute left-2.5 top-1/2 transform -translate-y-1/2" />
                            <input
                                type="text"
                                value={keySearchTerm}
                                onChange={(e: any) => setKeySearchTerm(e.target.value)}
                                placeholder="Filter keys..."
                                className="w-full bg-background border border-border rounded-md pl-8 pr-3 py-1 text-xs text-text focus:outline-none focus:border-primary transition-colors"
                                autoComplete="off"
                                autoCorrect="off"
                                spellCheck="false"
                            />
                        </div>
                    )}
                </div>
                <div className="flex items-center gap-2">
                    <button
                        onClick={onClose}
                        className="px-3 py-1.5 text-xs font-medium text-gray-400 hover:text-text hover:bg-white/5 rounded transition-colors"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={handleSave}
                        disabled={saving || isStale}
                        title={isStale ? "Cannot save - tab is from a different context" : "Save changes"}
                        className="px-3 py-1.5 text-xs font-medium bg-primary text-white rounded hover:bg-blue-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                        {saving ? 'Saving...' : 'Save'}
                    </button>
                </div>
            </div>

            {/* Content Area */}
            <div className="flex-1 overflow-hidden">
                {mode === MODE_YAML ? (
                    <Editor
                        height="100%"
                        defaultLanguage="yaml"
                        value={yamlContent}
                        onChange={(value: any) => !isStale && setYamlContent(value || '')}
                        onMount={handleEditorDidMount}
                        theme="vs-dark"
                        options={{
                            automaticLayout: true,
                            readOnly: isStale,
                            scrollbar: {
                                vertical: 'auto',
                                horizontal: 'auto',
                            },
                        }}
                    />
                ) : (
                    <div className="h-full overflow-auto p-4">
                        <div className="space-y-2">
                            {configMapEntries
                                .filter((entry: any) => !keySearchTerm || entry.key.toLowerCase().includes(keySearchTerm.toLowerCase()))
                                .map((entry) => {
                                    const isCert = isPEMCertificate(entry.value);
                                    const certInfo = certInfoCache[entry.id];
                                    return (
                                        <div key={entry.id} className="flex items-start gap-2 bg-surface-light rounded-md p-3">
                                            <input
                                                type="text"
                                                value={entry.key}
                                                onChange={(e: any) => !isStale && handleKeyChange(entry.id, e.target.value)}
                                                disabled={isStale}
                                                className={`w-48 shrink-0 px-2 py-1.5 text-sm bg-background border border-border rounded text-gray-200 focus:outline-none focus:border-primary ${isStale ? 'opacity-60 cursor-not-allowed' : ''}`}
                                                placeholder="Key"
                                                autoComplete="off"
                                                autoCorrect="off"
                                                autoCapitalize="off"
                                                spellCheck="false"
                                            />
                                            <div className="flex-1 min-w-0">
                                                <textarea
                                                    value={entry.value}
                                                    onChange={(e: any) => !isStale && handleValueChange(entry.id, e.target.value)}
                                                    disabled={isStale}
                                                    className={`w-full min-h-[60px] px-2 py-1.5 text-sm bg-background border border-border rounded text-gray-200 font-mono focus:outline-none focus:border-primary resize-y ${isStale ? 'opacity-60 cursor-not-allowed' : ''}`}
                                                    placeholder="Value"
                                                    autoComplete="off"
                                                    autoCorrect="off"
                                                    autoCapitalize="off"
                                                    spellCheck="false"
                                                />
                                                {/* Certificate expiry badge */}
                                                {isCert && certInfo && (
                                                    <div className="mt-1.5">
                                                        <span className={`inline-flex items-center gap-1.5 px-2 py-0.5 text-xs rounded border ${getExpiryBadgeStyle(certInfo)}`}>
                                                            <ShieldCheckIcon className="h-3 w-3" />
                                                            <span>{getExpiryText(certInfo)}</span>
                                                            {getCertSummary(certInfo) && (
                                                                <span className="text-gray-400">({getCertSummary(certInfo)})</span>
                                                            )}
                                                        </span>
                                                    </div>
                                                )}
                                            </div>
                                            <div className="flex flex-col gap-1 shrink-0">
                                                {isCert && (
                                                    <button
                                                        onClick={() => handleViewCertificate(entry)}
                                                        className="p-1.5 text-blue-400 hover:text-blue-300 hover:bg-blue-900/20 rounded transition-colors"
                                                        title="View certificate details"
                                                    >
                                                        <ShieldCheckIcon className="h-4 w-4" />
                                                    </button>
                                                )}
                                                <button
                                                    onClick={() => !isStale && handleDeleteEntry(entry.id)}
                                                    disabled={isStale}
                                                    className={`p-1.5 text-red-400 hover:text-red-300 hover:bg-red-900/20 rounded transition-colors ${isStale ? 'opacity-50 cursor-not-allowed' : ''}`}
                                                    title="Delete key"
                                                >
                                                    <TrashIcon className="h-4 w-4" />
                                                </button>
                                            </div>
                                        </div>
                                    );
                                })}
                            {configMapEntries.length === 0 && (
                                <div className="text-gray-500 text-sm text-center py-8">
                                    No configmap data. Click "Add Key" to create one.
                                </div>
                            )}
                            {configMapEntries.length > 0 && keySearchTerm && configMapEntries.filter((e: any) => e.key.toLowerCase().includes(keySearchTerm.toLowerCase())).length === 0 && (
                                <div className="text-gray-500 text-sm text-center py-8">
                                    No keys match "{keySearchTerm}"
                                </div>
                            )}
                        </div>
                        {!isStale && (
                            <button
                                onClick={handleAddKey}
                                className="mt-4 flex items-center gap-1.5 px-3 py-1.5 text-sm text-gray-400 hover:text-white bg-surface-light hover:bg-surface-hover rounded transition-colors"
                            >
                                <PlusIcon className="h-4 w-4" />
                                Add Key
                            </button>
                        )}
                    </div>
                )}
            </div>

            {/* Certificate Details Modal */}
            {selectedCert && (
                <CertificateModal
                    certificates={selectedCert.certificates}
                    pemData={selectedCert.pemData}
                    onClose={() => setSelectedCert(null)}
                />
            )}
        </div>
    );
}
