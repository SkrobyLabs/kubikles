import React, { useState, useEffect, useRef } from 'react';
import { GetPodLogs } from '../../wailsjs/go/main/App';
import Convert from 'ansi-to-html';

const converter = new Convert({
    fg: '#FFF',
    bg: '#1e1e1e',
    newline: true,
    escapeXML: true
});

export default function LogViewer({ namespace, pod }) {
    const [logs, setLogs] = useState('');
    const [loading, setLoading] = useState(false);
    const logsEndRef = useRef(null);

    useEffect(() => {
        if (namespace && pod) {
            fetchLogs();
        }
    }, [namespace, pod]);

    const fetchLogs = async () => {
        setLoading(true);
        try {
            const logData = await GetPodLogs(namespace, pod);
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
        <div className="h-full w-full bg-[#1e1e1e] text-gray-300 font-mono text-xs overflow-auto p-4">
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
    );
}
