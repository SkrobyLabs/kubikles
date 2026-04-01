import React, { useState, useEffect, useCallback } from 'react';
import {
    ArrowLeftIcon,
    PlusIcon,
    XMarkIcon,
    CommandLineIcon,
    KeyIcon,
    UserIcon,
} from '@heroicons/react/24/outline';
import {
    GetFullContextDetail,
    UpdateContextDetail,
} from 'wailsjs/go/main/App';
import { useNotification } from '~/context';

interface ExecEnvVar {
    name: string;
    value: string;
}

interface ClusterDetail {
    server: string;
    certificateAuthority: string;
    insecureSkipTLSVerify: boolean;
    proxyURL: string;
    tlsServerName: string;
    disableCompression: boolean;
}

interface AuthDetail {
    clientCertificate: string;
    clientKey: string;
    token: string;
    tokenFile: string;
    username: string;
    impersonate: string;
    impersonateGroups: string[];
    hasExecProvider: boolean;
    execAPIVersion: string;
    execCommand: string;
    execArgs: string[];
    execEnv: ExecEnvVar[];
    execInstallHint: string;
    execProvideCluster: boolean;
    hasAuthProvider: boolean;
    authProviderName: string;
}

interface FullContextDetail {
    name: string;
    cluster: string;
    authInfo: string;
    namespace: string;
    isActive: boolean;
    clusterDetail: ClusterDetail;
    authDetail: AuthDetail;
}

interface ContextEditorProps {
    contextName: string;
    onBack: () => void;
    onSaved: () => void;
}

type Tab = 'context' | 'cluster' | 'auth' | 'exec';

// --- Reusable field components ---

function TextField({ label, value, onChange, placeholder, readOnly, mono }: {
    label: string;
    value: string;
    onChange?: (v: string) => void;
    placeholder?: string;
    readOnly?: boolean;
    mono?: boolean;
}) {
    return (
        <div>
            <label className="block text-xs text-gray-400 mb-1">{label}</label>
            <input
                type="text"
                value={value}
                onChange={e => onChange?.(e.target.value)}
                placeholder={placeholder}
                readOnly={readOnly}
                className={`w-full bg-surface border border-border rounded px-2.5 py-1.5 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-primary ${readOnly ? 'opacity-60 cursor-default' : ''} ${mono ? 'font-mono text-xs' : ''}`}
            />
        </div>
    );
}

function ToggleField({ label, description, checked, onChange }: {
    label: string;
    description?: string;
    checked: boolean;
    onChange: (v: boolean) => void;
}) {
    return (
        <div className="flex items-start gap-3">
            <button
                onClick={() => onChange(!checked)}
                className={`relative inline-flex h-5 w-9 shrink-0 items-center rounded-full transition-colors mt-0.5 ${checked ? 'bg-primary' : 'bg-gray-600'}`}
            >
                <span className={`inline-block h-3.5 w-3.5 rounded-full bg-white transition-transform ${checked ? 'translate-x-[18px]' : 'translate-x-[3px]'}`} />
            </button>
            <div>
                <span className="text-sm text-white">{label}</span>
                {description && <p className="text-xs text-gray-500 mt-0.5">{description}</p>}
            </div>
        </div>
    );
}

function InfoPill({ label, value }: { label: string; value: string }) {
    return (
        <div className="flex items-center gap-2 px-2.5 py-1.5 bg-surface rounded border border-border min-w-0">
            <span className="text-xs text-gray-400 shrink-0">{label}:</span>
            <span className="text-xs text-white font-mono truncate">{value}</span>
        </div>
    );
}

// --- Tab bar ---

const TABS: { key: Tab; label: string }[] = [
    { key: 'context', label: 'Context' },
    { key: 'cluster', label: 'Cluster' },
    { key: 'auth', label: 'Auth' },
    { key: 'exec', label: 'Exec / Commands' },
];

export default function ContextEditor({ contextName, onBack, onSaved }: ContextEditorProps) {
    const [detail, setDetail] = useState<FullContextDetail | null>(null);
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [dirty, setDirty] = useState(false);
    const [tab, setTab] = useState<Tab>('context');
    const { addNotification } = useNotification();

    // --- Editable state ---
    // Context
    const [namespace, setNamespace] = useState('');
    // Cluster
    const [server, setServer] = useState('');
    const [certAuthority, setCertAuthority] = useState('');
    const [skipTLS, setSkipTLS] = useState(false);
    const [proxyURL, setProxyURL] = useState('');
    const [tlsServerName, setTlsServerName] = useState('');
    const [disableCompression, setDisableCompression] = useState(false);
    // Auth
    const [clientCert, setClientCert] = useState('');
    const [clientKey, setClientKey] = useState('');
    const [token, setToken] = useState('');
    const [tokenFile, setTokenFile] = useState('');
    const [username, setUsername] = useState('');
    const [impersonate, setImpersonate] = useState('');
    // Exec
    const [execCommand, setExecCommand] = useState('');
    const [execArgs, setExecArgs] = useState<string[]>([]);
    const [execEnv, setExecEnv] = useState<ExecEnvVar[]>([]);
    const [execInstallHint, setExecInstallHint] = useState('');
    const [execAPIVersion, setExecAPIVersion] = useState('');

    const loadDetail = useCallback(async () => {
        try {
            const d = await GetFullContextDetail(contextName);
            setDetail(d);
            setNamespace(d.namespace || '');
            setServer(d.clusterDetail?.server || '');
            setCertAuthority(d.clusterDetail?.certificateAuthority || '');
            setSkipTLS(d.clusterDetail?.insecureSkipTLSVerify || false);
            setProxyURL(d.clusterDetail?.proxyURL || '');
            setTlsServerName(d.clusterDetail?.tlsServerName || '');
            setDisableCompression(d.clusterDetail?.disableCompression || false);
            setClientCert(d.authDetail?.clientCertificate || '');
            setClientKey(d.authDetail?.clientKey || '');
            setToken(d.authDetail?.token || '');
            setTokenFile(d.authDetail?.tokenFile || '');
            setUsername(d.authDetail?.username || '');
            setImpersonate(d.authDetail?.impersonate || '');
            setExecCommand(d.authDetail?.execCommand || '');
            setExecArgs(d.authDetail?.execArgs || []);
            setExecEnv(d.authDetail?.execEnv || []);
            setExecInstallHint(d.authDetail?.execInstallHint || '');
            setExecAPIVersion(d.authDetail?.execAPIVersion || '');
            setDirty(false);
            // Auto-select exec tab if exec provider is configured
            if (d.authDetail?.hasExecProvider) {
                setTab('exec');
            }
        } catch (err: any) {
            addNotification({ type: 'error', title: 'Failed to load context details', message: String(err) });
        } finally {
            setLoading(false);
        }
    }, [contextName, addNotification]);

    useEffect(() => { loadDetail(); }, [loadDetail]);

    const markDirty = () => setDirty(true);
    const updateField = <T,>(setter: React.Dispatch<React.SetStateAction<T>>) => (value: T) => {
        setter(value);
        markDirty();
    };

    // --- Exec args helpers ---
    const addExecArg = () => { setExecArgs([...execArgs, '']); markDirty(); };
    const updateExecArg = (i: number, v: string) => {
        const next = [...execArgs]; next[i] = v; setExecArgs(next); markDirty();
    };
    const removeExecArg = (i: number) => {
        setExecArgs(execArgs.filter((_, idx) => idx !== i)); markDirty();
    };

    // --- Exec env helpers ---
    const addExecEnvVar = () => { setExecEnv([...execEnv, { name: '', value: '' }]); markDirty(); };
    const updateExecEnvVar = (i: number, field: 'name' | 'value', v: string) => {
        const next = [...execEnv]; next[i] = { ...next[i], [field]: v }; setExecEnv(next); markDirty();
    };
    const removeExecEnvVar = (i: number) => {
        setExecEnv(execEnv.filter((_, idx) => idx !== i)); markDirty();
    };

    const handleSave = async () => {
        if (!detail) return;
        setSaving(true);
        try {
            const updates: Record<string, any> = {};
            const orig = detail;
            if (namespace !== (orig.namespace || '')) updates.namespace = namespace;
            if (server !== (orig.clusterDetail?.server || '')) updates.server = server;
            if (certAuthority !== (orig.clusterDetail?.certificateAuthority || '')) updates.certificateAuthority = certAuthority;
            if (skipTLS !== (orig.clusterDetail?.insecureSkipTLSVerify || false)) updates.insecureSkipTLSVerify = skipTLS;
            if (proxyURL !== (orig.clusterDetail?.proxyURL || '')) updates.proxyURL = proxyURL;
            if (tlsServerName !== (orig.clusterDetail?.tlsServerName || '')) updates.tlsServerName = tlsServerName;
            if (disableCompression !== (orig.clusterDetail?.disableCompression || false)) updates.disableCompression = disableCompression;
            if (clientCert !== (orig.authDetail?.clientCertificate || '')) updates.clientCertificate = clientCert;
            if (clientKey !== (orig.authDetail?.clientKey || '')) updates.clientKey = clientKey;
            if (token !== (orig.authDetail?.token || '')) updates.token = token;
            if (tokenFile !== (orig.authDetail?.tokenFile || '')) updates.tokenFile = tokenFile;
            if (username !== (orig.authDetail?.username || '')) updates.username = username;
            if (impersonate !== (orig.authDetail?.impersonate || '')) updates.impersonate = impersonate;

            // Check exec changes
            const origArgs = orig.authDetail?.execArgs || [];
            const origEnv = orig.authDetail?.execEnv || [];
            const execChanged =
                execCommand !== (orig.authDetail?.execCommand || '') ||
                execInstallHint !== (orig.authDetail?.execInstallHint || '') ||
                JSON.stringify(execArgs) !== JSON.stringify(origArgs) ||
                JSON.stringify(execEnv) !== JSON.stringify(origEnv);

            if (execChanged) {
                updates.setExec = true;
                updates.execCommand = execCommand;
                updates.execArgs = execArgs;
                updates.execEnv = execEnv;
                updates.execInstallHint = execInstallHint;
            }

            if (Object.keys(updates).length === 0) {
                setDirty(false);
                return;
            }

            await UpdateContextDetail(contextName, updates);
            addNotification({ type: 'success', title: 'Context updated', message: `Saved changes to "${contextName}"` });
            setDirty(false);
            onSaved();
            await loadDetail();
        } catch (err: any) {
            addNotification({ type: 'error', title: 'Failed to save context', message: String(err) });
        } finally {
            setSaving(false);
        }
    };

    if (loading) {
        return (
            <div className="flex items-center gap-2 text-sm text-gray-400 py-8 justify-center">
                <div className="animate-spin rounded-full h-4 w-4 border-b-2 border-primary" />
                Loading context details...
            </div>
        );
    }

    if (!detail) {
        return (
            <div className="text-sm text-red-400 py-4 text-center">
                Failed to load context details.
                <button onClick={onBack} className="ml-2 text-primary hover:underline">Go back</button>
            </div>
        );
    }

    const authMethod = detail.authDetail?.hasExecProvider
        ? 'Exec Plugin'
        : detail.authDetail?.hasAuthProvider
            ? 'Auth Provider'
            : detail.authDetail?.token || detail.authDetail?.tokenFile
                ? 'Token'
                : detail.authDetail?.clientCertificate
                    ? 'Client Certificate'
                    : detail.authDetail?.username
                        ? 'Basic Auth'
                        : 'None detected';

    return (
        <div className="flex flex-col h-full">
            {/* Header */}
            <div className="flex items-center gap-2 px-4 py-3 border-b border-border shrink-0">
                <button
                    onClick={onBack}
                    className="p-1 rounded hover:bg-white/10 text-gray-400 hover:text-white transition-colors"
                >
                    <ArrowLeftIcon className="h-4 w-4" />
                </button>
                <div className="flex-1 min-w-0">
                    <h2 className="text-sm font-medium text-white truncate">{contextName}</h2>
                    <div className="flex items-center gap-2 mt-0.5">
                        <span className="text-xs text-gray-500">Cluster: {detail.cluster}</span>
                        <span className="text-xs text-gray-600">|</span>
                        <span className="text-xs text-gray-500">Auth: {detail.authInfo}</span>
                    </div>
                </div>
                {dirty && <span className="text-xs text-yellow-500 shrink-0">Unsaved changes</span>}
            </div>

            {/* Tab bar */}
            <div className="flex border-b border-border shrink-0">
                {TABS.map(t => (
                    <button
                        key={t.key}
                        onClick={() => setTab(t.key)}
                        className={`px-4 py-2 text-xs font-medium transition-colors relative ${
                            tab === t.key
                                ? 'text-primary'
                                : 'text-gray-400 hover:text-white'
                        }`}
                    >
                        {t.label}
                        {tab === t.key && (
                            <div className="absolute bottom-0 left-0 right-0 h-0.5 bg-primary" />
                        )}
                    </button>
                ))}
            </div>

            {/* Tab content */}
            <div className="flex-1 overflow-y-auto p-4 space-y-3">
                {tab === 'context' && (
                    <>
                        <TextField
                            label="Default Namespace"
                            value={namespace}
                            onChange={updateField(setNamespace)}
                            placeholder="default"
                        />
                        <div className="grid grid-cols-2 gap-2">
                            <InfoPill label="Cluster" value={detail.cluster} />
                            <InfoPill label="User" value={detail.authInfo} />
                        </div>
                        <div className="pt-2">
                            <InfoPill label="Auth Method" value={authMethod} />
                        </div>
                    </>
                )}

                {tab === 'cluster' && (
                    <>
                        <TextField
                            label="Server URL"
                            value={server}
                            onChange={updateField(setServer)}
                            placeholder="https://kubernetes.default.svc"
                            mono
                        />
                        <TextField
                            label="Certificate Authority"
                            value={certAuthority}
                            onChange={updateField(setCertAuthority)}
                            placeholder="/path/to/ca.crt"
                            mono
                        />
                        <TextField
                            label="TLS Server Name"
                            value={tlsServerName}
                            onChange={updateField(setTlsServerName)}
                            placeholder="Override for TLS SNI"
                        />
                        <TextField
                            label="Proxy URL"
                            value={proxyURL}
                            onChange={updateField(setProxyURL)}
                            placeholder="http://proxy:8080"
                            mono
                        />
                        <ToggleField
                            label="Skip TLS Verification"
                            description="Disable server certificate verification (insecure)"
                            checked={skipTLS}
                            onChange={updateField(setSkipTLS)}
                        />
                        <ToggleField
                            label="Disable Compression"
                            description="Disable HTTP compression for API requests"
                            checked={disableCompression}
                            onChange={updateField(setDisableCompression)}
                        />
                    </>
                )}

                {tab === 'auth' && (
                    <>
                        <div className="flex items-center gap-2 mb-1">
                            <span className="text-xs text-gray-400">Detected method:</span>
                            <span className="text-xs font-medium text-primary">{authMethod}</span>
                        </div>

                        {detail.authDetail?.hasAuthProvider && (
                            <div className="p-2.5 rounded bg-surface border border-border mb-2">
                                <div className="flex items-center gap-1.5 text-xs text-gray-400 mb-2">
                                    <KeyIcon className="h-3.5 w-3.5" />
                                    Auth Provider (read-only)
                                </div>
                                <InfoPill label="Provider" value={detail.authDetail.authProviderName} />
                            </div>
                        )}

                        <TextField
                            label="Client Certificate"
                            value={clientCert}
                            onChange={updateField(setClientCert)}
                            placeholder="/path/to/client.crt"
                            mono
                        />
                        <TextField
                            label="Client Key"
                            value={clientKey}
                            onChange={updateField(setClientKey)}
                            placeholder="/path/to/client.key"
                            mono
                        />
                        <TextField
                            label="Bearer Token"
                            value={token}
                            onChange={updateField(setToken)}
                            placeholder="Token value"
                            mono
                        />
                        <TextField
                            label="Token File"
                            value={tokenFile}
                            onChange={updateField(setTokenFile)}
                            placeholder="/var/run/secrets/token"
                            mono
                        />
                        <TextField
                            label="Username"
                            value={username}
                            onChange={updateField(setUsername)}
                            placeholder="Username for basic auth"
                        />

                        <div className="pt-2 border-t border-border">
                            <div className="flex items-center gap-1.5 text-xs text-gray-400 mb-2">
                                <UserIcon className="h-3.5 w-3.5" />
                                Impersonation
                            </div>
                            <TextField
                                label="Impersonate User"
                                value={impersonate}
                                onChange={updateField(setImpersonate)}
                                placeholder="user@example.com"
                            />
                            {detail.authDetail?.impersonateGroups?.length > 0 && (
                                <div className="mt-2">
                                    <label className="block text-xs text-gray-400 mb-1">Impersonate Groups</label>
                                    <div className="flex flex-wrap gap-1">
                                        {detail.authDetail.impersonateGroups.map(g => (
                                            <span key={g} className="px-2 py-0.5 text-xs bg-surface border border-border rounded text-gray-300">{g}</span>
                                        ))}
                                    </div>
                                </div>
                            )}
                        </div>
                    </>
                )}

                {tab === 'exec' && (
                    <>
                        <p className="text-xs text-gray-500 mb-2">
                            Configure the exec-based credential plugin used to authenticate to this cluster (e.g. aws-iam-authenticator, gcloud, kubelogin).
                        </p>

                        <TextField
                            label="Command"
                            value={execCommand}
                            onChange={updateField(setExecCommand)}
                            placeholder="aws-iam-authenticator"
                            mono
                        />
                        <TextField
                            label="API Version"
                            value={execAPIVersion}
                            onChange={updateField(setExecAPIVersion)}
                            placeholder="client.authentication.k8s.io/v1beta1"
                            mono
                        />
                        <TextField
                            label="Install Hint"
                            value={execInstallHint}
                            onChange={updateField(setExecInstallHint)}
                            placeholder="brew install aws-iam-authenticator"
                        />

                        {/* Arguments */}
                        <div>
                            <div className="flex items-center justify-between mb-1">
                                <label className="text-xs text-gray-400">Arguments</label>
                                <button
                                    onClick={addExecArg}
                                    className="flex items-center gap-1 text-xs text-gray-400 hover:text-white transition-colors"
                                >
                                    <PlusIcon className="h-3 w-3" /> Add
                                </button>
                            </div>
                            {execArgs.length === 0 ? (
                                <p className="text-xs text-gray-600 italic">No arguments configured</p>
                            ) : (
                                <div className="space-y-1">
                                    {execArgs.map((arg, i) => (
                                        <div key={i} className="flex items-center gap-1">
                                            <span className="text-xs text-gray-600 w-5 text-right shrink-0">{i}:</span>
                                            <input
                                                type="text"
                                                value={arg}
                                                onChange={e => updateExecArg(i, e.target.value)}
                                                className="flex-1 bg-surface border border-border rounded px-2 py-1 text-xs text-white font-mono placeholder-gray-600 focus:outline-none focus:border-primary"
                                                placeholder={`arg ${i}`}
                                            />
                                            <button
                                                onClick={() => removeExecArg(i)}
                                                className="p-0.5 rounded hover:bg-red-500/10 text-gray-500 hover:text-red-400 transition-colors"
                                            >
                                                <XMarkIcon className="h-3.5 w-3.5" />
                                            </button>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>

                        {/* Environment Variables */}
                        <div>
                            <div className="flex items-center justify-between mb-1">
                                <label className="text-xs text-gray-400">Environment Variables</label>
                                <button
                                    onClick={addExecEnvVar}
                                    className="flex items-center gap-1 text-xs text-gray-400 hover:text-white transition-colors"
                                >
                                    <PlusIcon className="h-3 w-3" /> Add
                                </button>
                            </div>
                            {execEnv.length === 0 ? (
                                <p className="text-xs text-gray-600 italic">No environment variables configured</p>
                            ) : (
                                <div className="space-y-1">
                                    {execEnv.map((ev, i) => (
                                        <div key={i} className="flex items-center gap-1">
                                            <input
                                                type="text"
                                                value={ev.name}
                                                onChange={e => updateExecEnvVar(i, 'name', e.target.value)}
                                                className="w-1/3 bg-surface border border-border rounded px-2 py-1 text-xs text-white font-mono placeholder-gray-600 focus:outline-none focus:border-primary"
                                                placeholder="NAME"
                                            />
                                            <span className="text-xs text-gray-600">=</span>
                                            <input
                                                type="text"
                                                value={ev.value}
                                                onChange={e => updateExecEnvVar(i, 'value', e.target.value)}
                                                className="flex-1 bg-surface border border-border rounded px-2 py-1 text-xs text-white font-mono placeholder-gray-600 focus:outline-none focus:border-primary"
                                                placeholder="value"
                                            />
                                            <button
                                                onClick={() => removeExecEnvVar(i)}
                                                className="p-0.5 rounded hover:bg-red-500/10 text-gray-500 hover:text-red-400 transition-colors"
                                            >
                                                <XMarkIcon className="h-3.5 w-3.5" />
                                            </button>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>

                        {/* Preview */}
                        {execCommand && (
                            <div className="pt-2 border-t border-border">
                                <label className="block text-xs text-gray-400 mb-1">
                                    <CommandLineIcon className="h-3.5 w-3.5 inline mr-1" />
                                    Preview
                                </label>
                                <div className="bg-surface border border-border rounded px-2.5 py-2 font-mono text-xs text-green-400 whitespace-pre-wrap break-all">
                                    {execEnv.filter(e => e.name).map(e => `${e.name}=${e.value} `).join('\\\n  ')}
                                    {execEnv.filter(e => e.name).length > 0 ? '\\\n  ' : ''}
                                    {execCommand}{execArgs.length > 0 ? ' ' + execArgs.join(' ') : ''}
                                </div>
                            </div>
                        )}
                    </>
                )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between px-4 py-3 border-t border-border shrink-0">
                <button
                    onClick={onBack}
                    className="px-3 py-1.5 text-sm text-gray-400 hover:text-white hover:bg-surface-hover rounded transition-colors"
                >
                    Back
                </button>
                <button
                    onClick={handleSave}
                    disabled={!dirty || saving}
                    className="px-4 py-1.5 text-sm rounded bg-primary hover:bg-primary/80 text-white transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                >
                    {saving ? 'Saving...' : 'Save Changes'}
                </button>
            </div>
        </div>
    );
}
