import React, { useState, useEffect, useCallback } from 'react';
import { ChevronDownIcon, ChevronRightIcon, WrenchScrewdriverIcon } from '@heroicons/react/24/outline';
import { GetDebugClusterConfig, SetDebugClusterConfig, ResetDebugCluster } from 'wailsjs/go/main/App';

interface DebugClusterConfig {
    namespaces: number;
    pods: number;
    deployments: number;
    services: number;
    configMaps: number;
    secrets: number;
    nodes: number;
    statefulSets: number;
    daemonSets: number;
    jobs: number;
    replicaSets: number;
}

const FIELDS: { key: keyof DebugClusterConfig; label: string }[] = [
    { key: 'namespaces', label: 'Namespaces' },
    { key: 'nodes', label: 'Nodes' },
    { key: 'pods', label: 'Pods/ns' },
    { key: 'deployments', label: 'Deploys' },
    { key: 'services', label: 'Services' },
    { key: 'configMaps', label: 'CMs' },
    { key: 'secrets', label: 'Secrets' },
    { key: 'statefulSets', label: 'STS' },
    { key: 'daemonSets', label: 'DaemonSets' },
    { key: 'jobs', label: 'Jobs' },
    { key: 'replicaSets', label: 'ReplicaSets' },
];

function estimateTotal(cfg: DebugClusterConfig): number {
    const ns = cfg.namespaces;
    // Each deployment creates 1 deploy + 1 RS + 3 pods
    const perNs =
        cfg.deployments * 5 +
        cfg.replicaSets +
        cfg.statefulSets * 4 + // 1 STS + 3 pods
        cfg.daemonSets +
        cfg.pods +
        cfg.services +
        cfg.configMaps +
        cfg.secrets +
        cfg.jobs +
        10; // hardcoded events per namespace
    return cfg.nodes + ns + ns * perNs;
}

export default function DebugClusterPanel() {
    const [expanded, setExpanded] = useState(false);
    const [config, setConfig] = useState<DebugClusterConfig | null>(null);
    const [applying, setApplying] = useState(false);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        GetDebugClusterConfig()
            .then((cfg: any) => setConfig(cfg))
            .catch(() => {});
    }, []);

    const handleChange = useCallback((key: keyof DebugClusterConfig, value: string) => {
        const num = parseInt(value, 10);
        if (isNaN(num) || num < 0) return;
        setConfig(prev => prev ? { ...prev, [key]: num } : prev);
    }, []);

    const handleApply = useCallback(async () => {
        if (!config) return;
        setApplying(true);
        setError(null);
        try {
            await SetDebugClusterConfig(config);
        } catch (err: any) {
            setError(err?.message || 'Failed to apply');
        } finally {
            setApplying(false);
        }
    }, [config]);

    const handleReset = useCallback(async () => {
        setApplying(true);
        setError(null);
        try {
            await ResetDebugCluster();
            const cfg = await GetDebugClusterConfig();
            setConfig(cfg as any);
        } catch (err: any) {
            setError(err?.message || 'Failed to reset');
        } finally {
            setApplying(false);
        }
    }, []);

    if (!config) return null;

    const total = estimateTotal(config);

    return (
        <div className="border-b border-border">
            <button
                onClick={() => setExpanded(e => !e)}
                className="w-full px-4 py-2 flex items-center gap-2 text-xs font-semibold text-amber-400 hover:bg-white/5 transition-colors"
            >
                <WrenchScrewdriverIcon className="h-3.5 w-3.5" />
                <span className="uppercase tracking-wider">Debug Cluster</span>
                <span className="ml-auto">
                    {expanded
                        ? <ChevronDownIcon className="h-3.5 w-3.5" />
                        : <ChevronRightIcon className="h-3.5 w-3.5" />
                    }
                </span>
            </button>
            {expanded && (
                <div className="px-4 pb-3 space-y-2">
                    <div className="grid grid-cols-2 gap-x-3 gap-y-1.5">
                        {FIELDS.map(({ key, label }) => (
                            <label key={key} className="flex items-center gap-1.5 text-xs text-gray-400">
                                <span className="w-[70px] truncate" title={label}>{label}</span>
                                <input
                                    type="number"
                                    min={0}
                                    value={config[key]}
                                    onChange={e => handleChange(key, e.target.value)}
                                    className="w-full bg-surface-secondary border border-border rounded px-1.5 py-0.5 text-xs text-text tabular-nums focus:outline-none focus:border-primary"
                                />
                            </label>
                        ))}
                    </div>
                    <div className="text-[10px] text-gray-500">
                        ≈ {total.toLocaleString()} total objects
                    </div>
                    {error && (
                        <div className="text-[10px] text-red-400 truncate" title={error}>{error}</div>
                    )}
                    <div className="flex gap-2">
                        <button
                            onClick={handleApply}
                            disabled={applying}
                            className="flex-1 px-2 py-1 text-xs font-medium rounded bg-primary/20 text-primary hover:bg-primary/30 disabled:opacity-50 transition-colors"
                        >
                            {applying ? 'Applying…' : 'Apply'}
                        </button>
                        <button
                            onClick={handleReset}
                            disabled={applying}
                            className="flex-1 px-2 py-1 text-xs font-medium rounded bg-white/5 text-gray-400 hover:bg-white/10 hover:text-text disabled:opacity-50 transition-colors"
                        >
                            Reset
                        </button>
                    </div>
                </div>
            )}
        </div>
    );
}
