import React, { useState, useEffect, useRef } from 'react';
import { GetPodLogs, GetAllPodLogs, SavePodLogs, SaveLogsBundle } from '../../../wailsjs/go/main/App';
import Convert from 'ansi-to-html';

const converter = new Convert({
    fg: '#FFF',
    bg: '#1e1e1e',
    newline: true,
    escapeXML: true
});

// Fix non-standard 4-digit 256-color codes (e.g., 0008 -> 8) to standard format
const normalizeAnsiCodes = (text) => {
    return text.replace(/\x1b\[38;5;0*(\d{1,3})m/g, '\x1b[38;5;$1m')
               .replace(/\x1b\[48;5;0*(\d{1,3})m/g, '\x1b[48;5;$1m');
};

import SearchSelect from './SearchSelect';

export default function LogViewer({ namespace, pod, containers = [], siblingPods = [], podContainerMap = {}, ownerName = '' }) {
    const [logs, setLogs] = useState('');
    const [loading, setLoading] = useState(false);
    const [downloading, setDownloading] = useState(false);
    const [downloadingBundle, setDownloadingBundle] = useState(false);
    const [selectedPod, setSelectedPod] = useState(pod);
    const [selectedContainer, setSelectedContainer] = useState(containers[0] || '');
    const [wrapLines, setWrapLines] = useState(true);
    const [showTimestamps, setShowTimestamps] = useState(false);
    const logsEndRef = useRef(null);

    useEffect(() => {
        if (namespace && selectedPod) {
            fetchLogs();
        }
    }, [namespace, selectedPod, selectedContainer, showTimestamps]);

    const fetchLogs = async () => {
        setLoading(true);
        try {
            const logData = await GetPodLogs(namespace, selectedPod, selectedContainer, showTimestamps);
            setLogs(logData);
        } catch (err) {
            setLogs(`Error fetching logs: ${err}`);
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        if (logsEndRef.current) {
            logsEndRef.current.scrollIntoView({ behavior: "smooth" });
        }
    }, [logs]);

    const getHtmlLogs = () => {
        if (!logs) return { __html: "No logs available." };
        return { __html: converter.toHtml(normalizeAnsiCodes(logs)) };
    };

    const downloadLogs = async () => {
        setDownloading(true);
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const filename = `${selectedPod}-${timestamp}.log`;
        try {
            const allLogs = await GetAllPodLogs(namespace, selectedPod, selectedContainer, showTimestamps);
            await SavePodLogs(allLogs, filename);
        } catch (err) {
            console.error('Failed to save logs:', err);
        } finally {
            setDownloading(false);
        }
    };

    const downloadBundle = async () => {
        setDownloadingBundle(true);
        const timestamp = new Date().toISOString().replace(/[:.]/g, '-');
        const bundleName = ownerName || pod;
        const filename = `${bundleName}-${timestamp}.zip`;

        try {
            const entries = [];
            const podsToDownload = siblingPods.length > 0 ? siblingPods : [pod];

            for (const podName of podsToDownload) {
                // Get containers for this pod from the map, or use current containers as fallback
                const podContainers = podContainerMap[podName] || containers;

                for (const containerName of podContainers) {
                    try {
                        const logs = await GetAllPodLogs(namespace, podName, containerName, showTimestamps);
                        entries.push({
                            podName,
                            containerName,
                            logs: logs || ''
                        });
                    } catch (err) {
                        console.error(`Failed to get logs for ${podName}/${containerName}:`, err);
                        entries.push({
                            podName,
                            containerName,
                            logs: `Error fetching logs: ${err}`
                        });
                    }
                }
            }

            await SaveLogsBundle(entries, filename);
        } catch (err) {
            console.error('Failed to save logs bundle:', err);
        } finally {
            setDownloadingBundle(false);
        }
    };

    return (
        <div className="flex flex-col h-full bg-[#1e1e1e]">
            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-500">Pod:</span>
                        <div className="w-64">
                            <SearchSelect
                                options={siblingPods.length > 0 ? siblingPods : [pod]}
                                value={selectedPod}
                                onChange={setSelectedPod}
                                placeholder="Select Pod..."
                                className="text-xs"
                            />
                        </div>
                    </div>
                    {containers.length > 0 && (
                        <div className="flex items-center gap-2">
                            <span className="text-xs text-gray-500">Container:</span>
                            <div className="w-48">
                                <SearchSelect
                                    options={containers}
                                    value={selectedContainer}
                                    onChange={setSelectedContainer}
                                    placeholder="Select Container..."
                                    className="text-xs"
                                />
                            </div>
                        </div>
                    )}
                </div>
                <div className="flex items-center gap-4">
                    <label className="flex items-center gap-1.5 text-xs text-gray-400 cursor-pointer select-none">
                        <input
                            type="checkbox"
                            checked={wrapLines}
                            onChange={(e) => setWrapLines(e.target.checked)}
                            className="w-3.5 h-3.5 rounded border-border bg-surface accent-primary cursor-pointer"
                        />
                        Wrap lines
                    </label>
                    <label className="flex items-center gap-1.5 text-xs text-gray-400 cursor-pointer select-none">
                        <input
                            type="checkbox"
                            checked={showTimestamps}
                            onChange={(e) => setShowTimestamps(e.target.checked)}
                            className="w-3.5 h-3.5 rounded border-border bg-surface accent-primary cursor-pointer"
                        />
                        Timestamps
                    </label>
                    <div className="w-px h-4 bg-border" />
                    <button
                        onClick={downloadLogs}
                        disabled={!logs || loading || downloading}
                        className="flex items-center gap-1 px-3 py-1.5 text-xs bg-surface border border-border rounded hover:bg-white/10 disabled:opacity-50 disabled:cursor-not-allowed"
                        title="Download current container logs"
                    >
                        {downloading ? (
                            <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                                <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                                <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                            </svg>
                        ) : (
                            <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M4 16v1a3 3 0 003 3h10a3 3 0 003-3v-1m-4-4l-4 4m0 0l-4-4m4 4V4" />
                            </svg>
                        )}
                        {downloading ? 'Downloading...' : 'Download'}
                    </button>
                    {siblingPods.length > 1 && (
                        <button
                            onClick={downloadBundle}
                            disabled={loading || downloadingBundle}
                            className="flex items-center gap-1 px-3 py-1.5 text-xs bg-surface border border-border rounded hover:bg-white/10 disabled:opacity-50 disabled:cursor-not-allowed"
                            title="Download all pods logs as zip"
                        >
                            {downloadingBundle ? (
                                <svg className="w-4 h-4 animate-spin" fill="none" viewBox="0 0 24 24">
                                    <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
                                    <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
                                </svg>
                            ) : (
                                <svg className="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                                    <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 8h14M5 8a2 2 0 110-4h14a2 2 0 110 4M5 8v10a2 2 0 002 2h10a2 2 0 002-2V8m-9 4h4" />
                                </svg>
                            )}
                            {downloadingBundle ? 'Bundling...' : 'Download All'}
                        </button>
                    )}
                </div>
            </div>

            {/* Logs Content */}
            <div className="flex-1 overflow-auto p-4 text-gray-300 font-mono text-xs">
                {loading ? (
                    <div className="flex items-center justify-center h-full">
                        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                    </div>
                ) : (
                    <div className={wrapLines ? "whitespace-pre-wrap break-all" : "whitespace-pre"}>
                        <div dangerouslySetInnerHTML={getHtmlLogs()} />
                        <div ref={logsEndRef} />
                    </div>
                )}
            </div>
        </div>
    );
}
