import React, { useState, useEffect, useRef } from 'react';
import { GetPodLogs } from '../../../wailsjs/go/main/App';
import Convert from 'ansi-to-html';

const converter = new Convert({
    fg: '#FFF',
    bg: '#1e1e1e',
    newline: true,
    escapeXML: true
});

import SearchSelect from './SearchSelect';

export default function LogViewer({ namespace, pod, containers = [] }) {
    const [logs, setLogs] = useState('');
    const [loading, setLoading] = useState(false);
    const [selectedContainer, setSelectedContainer] = useState(containers[0] || '');
    const logsEndRef = useRef(null);

    useEffect(() => {
        if (namespace && pod) {
            fetchLogs();
        }
    }, [namespace, pod, selectedContainer]);

    const fetchLogs = async () => {
        setLoading(true);
        try {
            const logData = await GetPodLogs(namespace, pod, selectedContainer);
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
        return { __html: converter.toHtml(logs) };
    };

    return (
        <div className="flex flex-col h-full bg-[#1e1e1e]">
            {/* Header Bar */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border bg-surface shrink-0">
                <div className="flex items-center gap-4">
                    <div className="text-sm font-medium text-gray-400">
                        {namespace}/{pod}
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
            </div>

            {/* Logs Content */}
            <div className="flex-1 overflow-auto p-4 text-gray-300 font-mono text-xs">
                {loading ? (
                    <div className="flex items-center justify-center h-full">
                        <div className="animate-spin rounded-full h-8 w-8 border-b-2 border-primary"></div>
                    </div>
                ) : (
                    <div className="whitespace-pre-wrap break-all">
                        <div dangerouslySetInnerHTML={getHtmlLogs()} />
                        <div ref={logsEndRef} />
                    </div>
                )}
            </div>
        </div>
    );
}
