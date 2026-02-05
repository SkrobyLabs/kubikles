import React, { useState, useMemo } from 'react';
import {
    CpuChipIcon,
    CircleStackIcon,
    ArrowPathIcon,
    SignalIcon,
    PlayIcon,
    PauseIcon,
    ChartBarIcon,
    BoltIcon,
    CommandLineIcon,
    DocumentTextIcon
} from '@heroicons/react/24/outline';

// Format bytes to human readable
const formatBytes = (bytes) => {
    if (bytes === 0) return '0 B';
    const k = 1024;
    const sizes = ['B', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
};

// Format nanoseconds to readable duration
const formatNanos = (ns) => {
    if (ns < 1000) return `${ns}ns`;
    if (ns < 1000000) return `${(ns / 1000).toFixed(2)}us`;
    if (ns < 1000000000) return `${(ns / 1000000).toFixed(2)}ms`;
    return `${(ns / 1000000000).toFixed(2)}s`;
};

// Mini sparkline chart component
const Sparkline = ({ data, color = 'stroke-primary', height = 32, width = 100 }) => {
    if (!data || data.length < 2) {
        return <div className="text-gray-600 text-xs italic">waiting...</div>;
    }

    const min = Math.min(...data);
    const max = Math.max(...data);
    const range = max - min || 1;

    const points = data.map((value, i) => {
        const x = (i / (data.length - 1)) * width;
        const y = height - ((value - min) / range) * (height - 4) - 2;
        return `${x},${y}`;
    }).join(' ');

    return (
        <svg width={width} height={height} className="overflow-visible">
            <polyline
                points={points}
                fill="none"
                className={color}
                strokeWidth="1.5"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
};

// Metric card component
const MetricCard = ({ title, value, subtitle, icon: Icon, sparklineData, sparklineColor }) => (
    <div className="bg-surface-light rounded-lg p-3 border border-border">
        <div className="flex items-center justify-between mb-1">
            <div className="flex items-center gap-1.5">
                {Icon && <Icon className="h-3.5 w-3.5 text-gray-500" />}
                <span className="text-xs font-medium text-gray-400">{title}</span>
            </div>
            {sparklineData && sparklineData.length > 1 && (
                <Sparkline data={sparklineData} color={sparklineColor} />
            )}
        </div>
        <div className="text-xl font-bold text-text">{value}</div>
        {subtitle && <div className="text-xs text-gray-500 mt-0.5">{subtitle}</div>}
    </div>
);

// Section component
const Section = ({ title, children }) => (
    <div className="mb-5">
        <h3 className="text-xs font-semibold text-gray-400 mb-2 uppercase tracking-wider">{title}</h3>
        <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-4 gap-2">
            {children}
        </div>
    </div>
);

export default function PerformancePanel({
    backendMetrics,
    metricsHistory,
    isPolling,
    onTogglePolling,
    bottomTabs = []
}) {
    const [activeTab, setActiveTab] = useState('overview');

    // Count keepAlive tabs by type
    const keepAliveCounts = useMemo(() => {
        const shells = bottomTabs.filter(t => t.keepAlive && t.id.startsWith('shell-')).length
            + bottomTabs.filter(t => t.keepAlive && t.id.startsWith('node-shell-')).length;
        const logs = bottomTabs.filter(t => t.keepAlive && (
            t.id.startsWith('logs-') ||
            t.id.startsWith('logs-deploy-') ||
            t.id.startsWith('logs-statefulset-') ||
            t.id.startsWith('logs-daemonset-') ||
            t.id.startsWith('logs-job-') ||
            t.id.startsWith('logs-cronjob-') ||
            t.id.startsWith('logs-replicaset-')
        )).length;
        return { shells, logs, total: shells + logs };
    }, [bottomTabs]);

    // Extract historical data for sparklines
    const heapHistory = useMemo(() =>
        metricsHistory.map(m => m?.memory?.heapInuse || 0),
        [metricsHistory]);

    const goroutineHistory = useMemo(() =>
        metricsHistory.map(m => m?.goroutines?.count || 0),
        [metricsHistory]);

    const watcherHistory = useMemo(() =>
        metricsHistory.map(m => m?.watchers?.active || 0),
        [metricsHistory]);

    if (!backendMetrics) {
        return (
            <div className="h-full flex items-center justify-center text-gray-500 bg-background">
                <div className="flex items-center gap-2">
                    <div className="animate-spin rounded-full h-5 w-5 border-b-2 border-primary"></div>
                    Loading metrics...
                </div>
            </div>
        );
    }

    const tabs = [
        { id: 'overview', label: 'Overview' },
        { id: 'activity', label: 'Activity' },
        { id: 'memory', label: 'Memory' },
        { id: 'watchers', label: 'Watchers' },
    ];

    return (
        <div className="h-full w-full bg-background flex flex-col overflow-hidden">
            {/* Header */}
            <div className="flex items-center justify-between px-4 py-2 border-b border-border shrink-0">
                <div className="flex items-center gap-4">
                    <div className="flex items-center gap-2">
                        <ChartBarIcon className="h-4 w-4 text-primary" />
                        <span className="text-sm font-semibold text-text">Performance</span>
                    </div>

                    {/* Tab Navigation */}
                    <div className="flex items-center gap-0.5 bg-surface-light rounded p-0.5">
                        {tabs.map(tab => (
                            <button
                                key={tab.id}
                                onClick={() => setActiveTab(tab.id)}
                                className={`px-2.5 py-1 text-xs font-medium rounded transition-colors ${activeTab === tab.id
                                        ? 'bg-primary text-white'
                                        : 'text-gray-400 hover:text-white'
                                    }`}
                            >
                                {tab.label}
                            </button>
                        ))}
                    </div>
                </div>

                <div className="flex items-center gap-3">
                    <span className={`text-xs ${isPolling ? 'text-green-400' : 'text-gray-500'}`}>
                        {isPolling ? 'Live' : 'Paused'}
                    </span>
                    <button
                        onClick={onTogglePolling}
                        className={`flex items-center gap-1 px-2 py-1 text-xs rounded transition-colors ${isPolling
                                ? 'bg-green-500/20 text-green-400 hover:bg-green-500/30'
                                : 'bg-gray-500/20 text-gray-400 hover:bg-gray-500/30'
                            }`}
                    >
                        {isPolling ? (
                            <>
                                <PauseIcon className="h-3 w-3" />
                                Pause
                            </>
                        ) : (
                            <>
                                <PlayIcon className="h-3 w-3" />
                                Resume
                            </>
                        )}
                    </button>
                </div>
            </div>

            {/* Content */}
            <div className="flex-1 overflow-auto p-4">
                {activeTab === 'overview' && (
                    <>
                        <Section title="Go Runtime">
                            <MetricCard
                                title="Heap Used"
                                icon={CircleStackIcon}
                                value={formatBytes(backendMetrics.memory?.heapInuse || 0)}
                                subtitle={`of ${formatBytes(backendMetrics.memory?.heapSys || 0)} sys`}
                                sparklineData={heapHistory}
                                sparklineColor="stroke-blue-400"
                            />
                            <MetricCard
                                title="Goroutines"
                                icon={CpuChipIcon}
                                value={backendMetrics.goroutines?.count || 0}
                                subtitle={`Peak: ${backendMetrics.goroutines?.maxObserved || 0}`}
                                sparklineData={goroutineHistory}
                                sparklineColor="stroke-green-400"
                            />
                            <MetricCard
                                title="GC Cycles"
                                icon={ArrowPathIcon}
                                value={backendMetrics.gc?.numGC || 0}
                                subtitle={`Last: ${formatNanos(backendMetrics.gc?.lastGCPauseNs || 0)}`}
                            />
                            <MetricCard
                                title="System Memory"
                                icon={CircleStackIcon}
                                value={formatBytes(backendMetrics.memory?.sys || 0)}
                                subtitle="Total from OS"
                            />
                        </Section>

                        <Section title="Active Resources">
                            <MetricCard
                                title="Watchers"
                                icon={SignalIcon}
                                value={backendMetrics.watchers?.active || 0}
                                subtitle={`Created: ${backendMetrics.watchers?.totalCreated || 0} / Cleaned: ${backendMetrics.watchers?.totalCleaned || 0}`}
                                sparklineData={watcherHistory}
                                sparklineColor="stroke-yellow-400"
                            />
                            <MetricCard
                                title="Port Forwards"
                                icon={SignalIcon}
                                value={backendMetrics.portForwards?.active || 0}
                                subtitle={`${backendMetrics.portForwards?.configs || 0} configs`}
                            />
                            <MetricCard
                                title="Log Streams"
                                icon={SignalIcon}
                                value={backendMetrics.logStreams?.active || 0}
                            />
                            <MetricCard
                                title="Ingress Forward"
                                icon={SignalIcon}
                                value={backendMetrics.ingressForwards?.active || 0}
                            />
                        </Section>

                        <Section title="Kept-Alive Tabs">
                            <MetricCard
                                title="Shell Tabs"
                                icon={CommandLineIcon}
                                value={keepAliveCounts.shells}
                                subtitle="WebSocket connections"
                            />
                            <MetricCard
                                title="Log Tabs"
                                icon={DocumentTextIcon}
                                value={keepAliveCounts.logs}
                                subtitle="Streaming connections"
                            />
                        </Section>

                        <Section title="Metrics Requests">
                            <MetricCard
                                title="Total"
                                icon={ChartBarIcon}
                                value={backendMetrics.metricsRequests?.total || 0}
                                subtitle="All-time requests"
                            />
                            <MetricCard
                                title="Pending"
                                icon={ArrowPathIcon}
                                value={backendMetrics.metricsRequests?.pending || 0}
                                subtitle="In-flight requests"
                            />
                            <MetricCard
                                title="Completed"
                                icon={ChartBarIcon}
                                value={backendMetrics.metricsRequests?.completed || 0}
                                subtitle="Successfully finished"
                            />
                            <MetricCard
                                title="Cancelled"
                                icon={BoltIcon}
                                value={backendMetrics.metricsRequests?.cancelled || 0}
                                subtitle="Stale requests cancelled"
                            />
                        </Section>

                        <Section title="List Requests">
                            <MetricCard
                                title="Total"
                                icon={ChartBarIcon}
                                value={backendMetrics.listRequests?.total || 0}
                                subtitle="All-time requests"
                            />
                            <MetricCard
                                title="Pending"
                                icon={ArrowPathIcon}
                                value={backendMetrics.listRequests?.pending || 0}
                                subtitle="In-flight requests"
                            />
                            <MetricCard
                                title="Completed"
                                icon={ChartBarIcon}
                                value={backendMetrics.listRequests?.completed || 0}
                                subtitle="Successfully finished"
                            />
                            <MetricCard
                                title="Cancelled"
                                icon={BoltIcon}
                                value={backendMetrics.listRequests?.cancelled || 0}
                                subtitle="Stale requests cancelled"
                            />
                        </Section>
                    </>
                )}

                {activeTab === 'activity' && (
                    <>
                        <Section title="Event Summary">
                            <MetricCard
                                title="Total Events"
                                icon={BoltIcon}
                                value={backendMetrics.activity?.totalEvents?.toLocaleString() || 0}
                                subtitle={`Since startup`}
                            />
                            <MetricCard
                                title="Tracking Duration"
                                icon={ChartBarIcon}
                                value={`${Math.floor((backendMetrics.activity?.windowDuration || 0) / 1000)}s`}
                                subtitle="Time window"
                            />
                        </Section>

                        <div className="mb-5">
                            <h3 className="text-xs font-semibold text-gray-400 mb-2 uppercase tracking-wider">
                                Top Event Sources (sorted by total events)
                            </h3>
                            <div className="bg-surface-light rounded-lg border border-border overflow-hidden">
                                {(backendMetrics.activity?.topWatchers || []).length === 0 ? (
                                    <div className="text-gray-500 text-sm p-4">No events recorded yet</div>
                                ) : (
                                    <table className="w-full text-xs">
                                        <thead>
                                            <tr className="border-b border-border bg-surface">
                                                <th className="text-left py-2 px-3 font-medium text-gray-400">Watcher</th>
                                                <th className="text-right py-2 px-2 font-medium text-gray-400 w-20">Added</th>
                                                <th className="text-right py-2 px-2 font-medium text-gray-400 w-20">Modified</th>
                                                <th className="text-right py-2 px-2 font-medium text-gray-400 w-20">Deleted</th>
                                                <th className="text-right py-2 px-2 font-medium text-gray-400 w-20">Total</th>
                                                <th className="text-right py-2 px-3 font-medium text-gray-400 w-24">Rate</th>
                                            </tr>
                                        </thead>
                                        <tbody>
                                            {backendMetrics.activity.topWatchers.map((watcher, i) => {
                                                const isHot = watcher.eventsPerSec > 1;
                                                const isWarm = watcher.eventsPerSec > 0.1;
                                                return (
                                                    <tr
                                                        key={watcher.key}
                                                        className={`border-b border-border/50 last:border-0 ${
                                                            isHot ? 'bg-red-500/10' : isWarm ? 'bg-yellow-500/5' : ''
                                                        }`}
                                                    >
                                                        <td className="py-2 px-3 font-mono text-gray-300">
                                                            <div className="flex items-center gap-2">
                                                                {isHot && <span className="w-2 h-2 rounded-full bg-red-500 animate-pulse" />}
                                                                {!isHot && isWarm && <span className="w-2 h-2 rounded-full bg-yellow-500" />}
                                                                {watcher.key}
                                                            </div>
                                                        </td>
                                                        <td className="py-2 px-2 text-right text-green-400">{watcher.added}</td>
                                                        <td className="py-2 px-2 text-right text-blue-400">{watcher.modified}</td>
                                                        <td className="py-2 px-2 text-right text-red-400">{watcher.deleted}</td>
                                                        <td className="py-2 px-2 text-right font-semibold text-white">{watcher.totalEvents}</td>
                                                        <td className={`py-2 px-3 text-right font-mono ${
                                                            isHot ? 'text-red-400 font-semibold' : isWarm ? 'text-yellow-400' : 'text-gray-500'
                                                        }`}>
                                                            {watcher.eventsPerSec.toFixed(2)}/s
                                                        </td>
                                                    </tr>
                                                );
                                            })}
                                        </tbody>
                                    </table>
                                )}
                            </div>
                            <p className="text-xs text-gray-600 mt-2">
                                Hot watchers (&gt;1 event/s) are highlighted in red. Warm watchers (&gt;0.1 event/s) in yellow.
                            </p>
                        </div>
                    </>
                )}

                {activeTab === 'memory' && (
                    <>
                        <Section title="Heap Memory">
                            <MetricCard title="Heap Alloc" value={formatBytes(backendMetrics.memory?.heapAlloc || 0)} />
                            <MetricCard title="Heap Sys" value={formatBytes(backendMetrics.memory?.heapSys || 0)} />
                            <MetricCard title="Heap Idle" value={formatBytes(backendMetrics.memory?.heapIdle || 0)} />
                            <MetricCard title="Heap In Use" value={formatBytes(backendMetrics.memory?.heapInuse || 0)} />
                            <MetricCard title="Heap Released" value={formatBytes(backendMetrics.memory?.heapReleased || 0)} />
                        </Section>

                        <Section title="Stack & Other">
                            <MetricCard title="Stack In Use" value={formatBytes(backendMetrics.memory?.stackInuse || 0)} />
                            <MetricCard title="Stack Sys" value={formatBytes(backendMetrics.memory?.stackSys || 0)} />
                            <MetricCard title="MSpan In Use" value={formatBytes(backendMetrics.memory?.mspanInuse || 0)} />
                            <MetricCard title="MCache In Use" value={formatBytes(backendMetrics.memory?.mcacheInuse || 0)} />
                        </Section>

                        <Section title="Totals">
                            <MetricCard title="Total Allocated" value={formatBytes(backendMetrics.memory?.totalAlloc || 0)} subtitle="Cumulative" />
                            <MetricCard title="System Memory" value={formatBytes(backendMetrics.memory?.sys || 0)} subtitle="From OS" />
                        </Section>

                        <Section title="Garbage Collection">
                            <MetricCard title="GC Cycles" value={backendMetrics.gc?.numGC || 0} />
                            <MetricCard title="Last GC Pause" value={formatNanos(backendMetrics.gc?.lastGCPauseNs || 0)} />
                            <MetricCard title="Total GC Pause" value={formatNanos(backendMetrics.gc?.totalPauseNs || 0)} />
                            <MetricCard title="Next GC Target" value={formatBytes(backendMetrics.gc?.nextGCBytes || 0)} />
                            <MetricCard
                                title="GC CPU Fraction"
                                value={`${((backendMetrics.gc?.gcCPUFraction || 0) * 100).toFixed(4)}%`}
                            />
                        </Section>
                    </>
                )}

                {activeTab === 'watchers' && (
                    <>
                        <Section title="Watcher Statistics">
                            <MetricCard title="Active" value={backendMetrics.watchers?.active || 0} />
                            <MetricCard title="Total Created" value={backendMetrics.watchers?.totalCreated || 0} />
                            <MetricCard title="Total Cleaned" value={backendMetrics.watchers?.totalCleaned || 0} />
                        </Section>

                        <div className="mb-5">
                            <h3 className="text-xs font-semibold text-gray-400 mb-2 uppercase tracking-wider">Active Watcher Keys</h3>
                            <div className="bg-surface-light rounded-lg border border-border p-3 max-h-64 overflow-auto">
                                {(backendMetrics.watchers?.watcherKeys || []).length === 0 ? (
                                    <div className="text-gray-500 text-sm">No active watchers</div>
                                ) : (
                                    <ul className="space-y-1 text-xs font-mono">
                                        {backendMetrics.watchers.watcherKeys.map((key, i) => (
                                            <li key={i} className="text-gray-300 py-1 border-b border-border/50 last:border-0">
                                                {key}
                                            </li>
                                        ))}
                                    </ul>
                                )}
                            </div>
                        </div>
                    </>
                )}
            </div>
        </div>
    );
}
