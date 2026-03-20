import { useState, useEffect, useRef, useCallback } from 'react';
import { StartEmbeddedBrowser, StopEmbeddedBrowser, GetEmbeddedBrowserStatus, SendTextToEmbeddedBrowser } from 'wailsjs/go/main/App';
import { useK8s } from '~/context';

type BrowserStatus = 'stopped' | 'starting' | 'running' | 'error';

interface BrowserSession {
    podName: string;
    namespace: string;
    localPort: number;
    status: BrowserStatus;
    error?: string;
}

export default function EmbeddedBrowser() {
    const { namespaces } = useK8s();
    const [selectedNamespace, setSelectedNamespace] = useState<string>('default');
    const [session, setSession] = useState<BrowserSession | null>(null);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);

    // iframeKey forces a remount (full reload) of the iframe when incremented.
    // We bump it once after the session is running (delayed) so KasmVNC gets
    // a clean load after the server is fully up — fixes the "must switch tab" issue.
    const [iframeKey, setIframeKey] = useState(0);
    const iframeRef = useRef<HTMLIFrameElement>(null);
    const reloadTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

    // Clipboard panel
    const [clipboardOpen, setClipboardOpen] = useState(false);
    const [clipboardText, setClipboardText] = useState('');
    const [clipboardStatus, setClipboardStatus] = useState<'idle' | 'ok' | 'err'>('idle');
    const clipboardTextareaRef = useRef<HTMLTextAreaElement>(null);

    const bumpIframe = useCallback(() => setIframeKey(k => k + 1), []);

    // On mount: restore any existing session
    useEffect(() => {
        GetEmbeddedBrowserStatus().then((s: BrowserSession) => {
            if (s.status !== 'stopped') setSession(s);
        }).catch(() => {});
    }, []);

    // When session becomes running, schedule a delayed iframe reload.
    // This gives KasmVNC ~2 s to finish starting, then loads the iframe fresh
    // so its WebSocket initialises properly without needing a manual tab switch.
    useEffect(() => {
        if (reloadTimerRef.current) clearTimeout(reloadTimerRef.current);
        if (session?.status === 'running') {
            reloadTimerRef.current = setTimeout(bumpIframe, 2000);
        }
        return () => {
            if (reloadTimerRef.current) clearTimeout(reloadTimerRef.current);
        };
    }, [session?.status]);

    // Namespace selector fallback
    useEffect(() => {
        if (namespaces.length > 0 && !namespaces.includes(selectedNamespace)) {
            setSelectedNamespace(namespaces[0]);
        }
    }, [namespaces]);

    // Clipboard panel focus
    useEffect(() => {
        if (clipboardOpen) setTimeout(() => clipboardTextareaRef.current?.focus(), 50);
        else { setClipboardText(''); setClipboardStatus('idle'); }
    }, [clipboardOpen]);

    const handleStart = async () => {
        setLoading(true);
        setError(null);
        try {
            const s = await StartEmbeddedBrowser(selectedNamespace);
            setSession(s as BrowserSession);
            if (s.status === 'error') setError(s.error || 'Failed to start browser');
        } catch (e: any) {
            setError(e?.message || String(e));
        } finally {
            setLoading(false);
        }
    };

    const handleStop = async () => {
        setLoading(true);
        setError(null);
        try {
            await StopEmbeddedBrowser();
            setSession(null);
            setClipboardOpen(false);
        } catch (e: any) {
            setError(e?.message || String(e));
        } finally {
            setLoading(false);
        }
    };

    const handleSendClipboard = async () => {
        if (!session || !clipboardText) return;
        setClipboardStatus('idle');
        try {
            await SendTextToEmbeddedBrowser(clipboardText);
            setClipboardStatus('ok');
            setTimeout(() => { setClipboardOpen(false); iframeRef.current?.focus(); }, 800);
        } catch {
            setClipboardStatus('err');
        }
    };

    const iframeUrl = session?.status === 'running' ? `http://localhost:${session.localPort}` : null;

    return (
        <div className="flex flex-col h-full bg-zinc-950 text-zinc-100">
            {/* Toolbar */}
            <div className="flex items-center gap-3 px-4 py-3 border-b border-zinc-800 bg-zinc-900 shrink-0">
                <span className="text-sm font-medium text-zinc-300 whitespace-nowrap">Embedded Browser</span>

                {session?.status !== 'running' && (
                    <>
                        <label className="text-xs text-zinc-400 whitespace-nowrap">Namespace</label>
                        <select
                            value={selectedNamespace}
                            onChange={e => setSelectedNamespace(e.target.value)}
                            disabled={loading}
                            className="text-xs bg-zinc-800 border border-zinc-700 text-zinc-200 rounded px-2 py-1 focus:outline-none focus:border-zinc-500"
                        >
                            {namespaces.length === 0 && <option value="default">default</option>}
                            {namespaces.map(ns => <option key={ns} value={ns}>{ns}</option>)}
                        </select>
                    </>
                )}

                {session?.status === 'running' && (
                    <span className="text-xs text-zinc-400">
                        Pod <span className="text-zinc-200 font-mono">{session.podName}</span>
                        {' '}in <span className="text-zinc-200 font-mono">{session.namespace}</span>
                        {' '}→ <span className="text-zinc-200 font-mono">localhost:{session.localPort}</span>
                    </span>
                )}

                <div className="ml-auto flex items-center gap-2">
                    {session?.status === 'running' && (
                        <>
                            <button
                                onClick={() => setClipboardOpen(v => !v)}
                                title="Paste text from your Mac into the remote browser"
                                className={`text-xs px-3 py-1 rounded transition-colors ${
                                    clipboardOpen
                                        ? 'bg-amber-700 text-amber-100'
                                        : 'bg-zinc-700 hover:bg-zinc-600 text-zinc-200'
                                }`}
                            >
                                Clipboard
                            </button>
                            <button
                                onClick={bumpIframe}
                                disabled={loading}
                                className="text-xs px-3 py-1 rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-200 transition-colors"
                            >
                                Reload
                            </button>
                            <button
                                onClick={handleStop}
                                disabled={loading}
                                className="text-xs px-3 py-1 rounded bg-red-900 hover:bg-red-800 text-red-200 transition-colors disabled:opacity-50"
                            >
                                {loading ? 'Stopping…' : 'Stop Browser'}
                            </button>
                        </>
                    )}
                    {session?.status !== 'running' && (
                        <button
                            onClick={handleStart}
                            disabled={loading || session?.status === 'starting'}
                            className="text-xs px-3 py-1 rounded bg-blue-700 hover:bg-blue-600 text-white transition-colors disabled:opacity-50"
                        >
                            {loading || session?.status === 'starting' ? 'Starting…' : 'Start Browser'}
                        </button>
                    )}
                </div>
            </div>

            {/* Clipboard panel */}
            {clipboardOpen && session?.status === 'running' && (
                <div className="px-4 py-3 border-b border-zinc-800 bg-zinc-900/80 shrink-0 flex flex-col gap-2">
                    <p className="text-xs text-zinc-400">
                        Paste your text here (⌘V), then click <span className="text-zinc-200">Send to Browser</span> — it will be available to paste (Ctrl+V) inside the remote Chromium.
                    </p>
                    <div className="flex gap-2 items-start">
                        <textarea
                            ref={clipboardTextareaRef}
                            value={clipboardText}
                            onChange={e => { setClipboardText(e.target.value); setClipboardStatus('idle'); }}
                            rows={3}
                            placeholder="Paste here…"
                            className="flex-1 text-xs bg-zinc-800 border border-zinc-700 text-zinc-200 rounded px-2 py-1.5 resize-none focus:outline-none focus:border-zinc-500 font-mono"
                        />
                        <div className="flex flex-col gap-1.5">
                            <button
                                onClick={handleSendClipboard}
                                disabled={!clipboardText}
                                className={`text-xs px-3 py-1.5 rounded transition-colors disabled:opacity-40 ${
                                    clipboardStatus === 'ok' ? 'bg-green-700 text-green-100'
                                    : clipboardStatus === 'err' ? 'bg-red-800 text-red-200'
                                    : 'bg-blue-700 hover:bg-blue-600 text-white'
                                }`}
                            >
                                {clipboardStatus === 'ok' ? 'Sent!' : clipboardStatus === 'err' ? 'Failed' : 'Send to Browser'}
                            </button>
                            <button
                                onClick={() => setClipboardOpen(false)}
                                className="text-xs px-3 py-1 rounded bg-zinc-700 hover:bg-zinc-600 text-zinc-300 transition-colors"
                            >
                                Cancel
                            </button>
                        </div>
                    </div>
                    {clipboardStatus === 'err' && (
                        <p className="text-xs text-red-400">
                            Failed to inject text (xclip/xdotool not available in pod). Use the browser's built-in clipboard panel instead (hover the left edge of the screen).
                        </p>
                    )}
                </div>
            )}

            {/* Error banner */}
            {error && (
                <div className="px-4 py-2 bg-red-950 border-b border-red-800 text-red-300 text-sm shrink-0">
                    {error}
                </div>
            )}

            {/* Content area */}
            <div className="flex-1 relative overflow-hidden">
                {iframeUrl ? (
                    <iframe
                        key={iframeKey}
                        ref={iframeRef}
                        src={iframeUrl}
                        className="absolute inset-0 w-full h-full border-0"
                        title="Embedded Browser"
                        allow="clipboard-read; clipboard-write"
                    />
                ) : (
                    <div className="flex flex-col items-center justify-center h-full gap-4 text-center px-8">
                        {session?.status === 'starting' || loading ? (
                            <>
                                <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
                                <p className="text-zinc-400 text-sm">
                                    Launching Chromium pod in namespace <span className="font-mono text-zinc-200">{selectedNamespace}</span>…
                                    <br />
                                    <span className="text-xs text-zinc-500">This may take up to a minute on first pull.</span>
                                </p>
                            </>
                        ) : (
                            <>
                                <div className="text-5xl opacity-20">🌐</div>
                                <div>
                                    <p className="text-zinc-300 text-sm font-medium mb-1">Embedded Browser</p>
                                    <p className="text-zinc-500 text-xs max-w-sm">
                                        Launches a Chromium pod inside your cluster with full access to cluster-internal
                                        services (e.g. <span className="font-mono">http://my-svc.default.svc.cluster.local</span>).
                                    </p>
                                </div>
                                <button
                                    onClick={handleStart}
                                    disabled={loading}
                                    className="mt-2 text-sm px-4 py-2 rounded bg-blue-700 hover:bg-blue-600 text-white transition-colors disabled:opacity-50"
                                >
                                    Start Browser
                                </button>
                            </>
                        )}
                    </div>
                )}
            </div>
        </div>
    );
}
