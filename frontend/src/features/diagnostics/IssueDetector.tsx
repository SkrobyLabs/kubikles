import React, { useCallback, useMemo, useRef } from 'react';
import { useK8s, useIssueDetector } from '~/context';
import type { Finding, ScanResult } from '~/hooks/useIssueDetector';
import Tooltip from '~/components/shared/Tooltip';
import {
    MagnifyingGlassIcon,
    ArrowPathIcon,
    XMarkIcon,
    ExclamationTriangleIcon,
    ExclamationCircleIcon,
    InformationCircleIcon,
    ChevronDownIcon,
    ChevronRightIcon,
    FolderOpenIcon,
    AdjustmentsHorizontalIcon,
    CheckIcon,
    BugAntIcon,
} from '@heroicons/react/24/outline';

const SEVERITY_CONFIG: Record<string, { label: string; color: string; bgColor: string; icon: typeof ExclamationCircleIcon }> = {
    critical: { label: 'Critical', color: 'text-red-400', bgColor: 'bg-red-900/30 border-red-800/50', icon: ExclamationCircleIcon },
    warning: { label: 'Warning', color: 'text-amber-400', bgColor: 'bg-amber-900/30 border-amber-800/50', icon: ExclamationTriangleIcon },
    info: { label: 'Info', color: 'text-blue-400', bgColor: 'bg-blue-900/30 border-blue-800/50', icon: InformationCircleIcon },
};

const CATEGORY_LABELS: Record<string, string> = {
    networking: 'Networking',
    workloads: 'Workloads',
    storage: 'Storage',
    security: 'Security',
    config: 'Config',
    deprecation: 'Deprecation',
};

export default function IssueDetector({ onClose }: { onClose?: () => void }) {
    const { namespaces } = useK8s();
    const {
        scanning, progress, result, rules, error,
        runScan, reloadRules, openRulesDir,
        selectedNamespaces, setSelectedNamespaces,
        selectedCategories, setSelectedCategories,
        clusterWide, setClusterWide,
        disabledRules, setDisabledRules,
        groupBy, setGroupBy,
        searchFilter, setSearchFilter,
        severityFilter, setSeverityFilter,
        expandedFindings, setExpandedFindings,
        expandedGroups, setExpandedGroups,
        expandedRules, setExpandedRules,
        expandedSubGroups, setExpandedSubGroups,
        showRulesPanel, setShowRulesPanel,
    } = useIssueDetector();

    // Track whether we already auto-expanded for the current result
    const autoExpandedForResult = useRef<ScanResult | null>(null);

    const handleScan = useCallback(() => {
        autoExpandedForResult.current = null;
        runScan(selectedNamespaces, selectedCategories, Array.from(disabledRules), clusterWide);
    }, [selectedNamespaces, selectedCategories, disabledRules, clusterWide, runScan]);

    const toggleRule = useCallback((ruleId: string) => {
        setDisabledRules(prev => {
            const next = new Set(prev);
            if (next.has(ruleId)) next.delete(ruleId);
            else next.add(ruleId);
            return next;
        });
    }, [setDisabledRules]);

    const toggleSeverity = useCallback((sev: string) => {
        setSeverityFilter(prev => {
            const next = new Set(prev);
            if (next.has(sev)) next.delete(sev);
            else next.add(sev);
            return next;
        });
    }, [setSeverityFilter]);

    const toggleRuleGroup = useCallback((key: string) => {
        setExpandedRules(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    }, [setExpandedRules]);

    const toggleSubGroup = useCallback((key: string) => {
        setExpandedSubGroups(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    }, [setExpandedSubGroups]);

    const toggleFinding = useCallback((key: string) => {
        setExpandedFindings(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    }, [setExpandedFindings]);

    const toggleGroup = useCallback((key: string) => {
        setExpandedGroups(prev => {
            const next = new Set(prev);
            if (next.has(key)) next.delete(key);
            else next.add(key);
            return next;
        });
    }, [setExpandedGroups]);

    // Filter and group findings
    const filteredFindings = useMemo(() => {
        if (!result?.findings) return [];
        return result.findings.filter(f => {
            if (!severityFilter.has(f.severity)) return false;
            if (searchFilter) {
                const q = searchFilter.toLowerCase();
                return (
                    f.ruleID.toLowerCase().includes(q) ||
                    f.ruleName.toLowerCase().includes(q) ||
                    f.description.toLowerCase().includes(q) ||
                    f.resource.name.toLowerCase().includes(q) ||
                    f.resource.kind.toLowerCase().includes(q)
                );
            }
            return true;
        });
    }, [result, severityFilter, searchFilter]);

    const grouped = useMemo(() => {
        // First level: group by selected groupBy key
        const groups: Record<string, Finding[]> = {};
        for (const f of filteredFindings) {
            let key: string;
            switch (groupBy) {
                case 'severity': key = f.severity; break;
                case 'category': key = f.category; break;
                case 'kind': key = f.resource.kind; break;
            }
            if (!groups[key]) groups[key] = [];
            groups[key].push(f);
        }

        // Sort group keys
        const sortOrder = groupBy === 'severity'
            ? ['critical', 'warning', 'info']
            : Object.keys(groups).sort();

        // Second level: sub-group by ruleID within each group
        // Third level (optional): sub-group by groupKey within a rule
        return sortOrder
            .filter(k => groups[k]?.length > 0)
            .map(k => {
                const byRule: Record<string, Finding[]> = {};
                for (const f of groups[k]) {
                    if (!byRule[f.ruleID]) byRule[f.ruleID] = [];
                    byRule[f.ruleID].push(f);
                }
                const ruleGroups = Object.entries(byRule).map(([ruleID, findings]) => {
                    // Sub-group by groupKey if present, otherwise by namespace
                    const hasGroupKeys = findings.some(f => f.groupKey);
                    const bySub: Record<string, Finding[]> = {};
                    for (const f of findings) {
                        const sk = hasGroupKeys
                            ? (f.groupKey || '')
                            : (f.resource.namespace || '(cluster)');
                        if (!bySub[sk]) bySub[sk] = [];
                        bySub[sk].push(f);
                    }
                    // Only use sub-groups if there's more than one group
                    const subEntries = Object.entries(bySub);
                    const subGroups = subEntries.length > 1
                        ? subEntries.map(([sk, fs]) => ({ key: sk, findings: fs }))
                        : undefined;

                    return {
                        ruleID,
                        ruleName: findings[0].ruleName,
                        severity: findings[0].severity,
                        findings,
                        subGroups,
                    };
                });
                return { key: k, totalFindings: groups[k].length, ruleGroups };
            });
    }, [filteredFindings, groupBy]);

    // Auto-expand groups when a new result comes in (once per result object)
    if (result && autoExpandedForResult.current !== result && grouped.length > 0) {
        autoExpandedForResult.current = result;
        setExpandedGroups(new Set(grouped.map(g => g.key)));
    }

    const severityCounts = useMemo(() => {
        if (!result?.findings) return { critical: 0, warning: 0, info: 0 };
        const counts = { critical: 0, warning: 0, info: 0 };
        for (const f of result.findings) {
            if (f.severity in counts) counts[f.severity as keyof typeof counts]++;
        }
        return counts;
    }, [result]);

    const groupLabel = (key: string): string => {
        if (groupBy === 'severity') return SEVERITY_CONFIG[key]?.label || key;
        if (groupBy === 'category') return CATEGORY_LABELS[key] || key;
        return key;
    };

    const renderFindings = (findings: Finding[], ruleKey: string) => (
        <>
            {findings.map((f, i) => {
                const fKey = `${ruleKey}-${f.resource.namespace}-${f.resource.name}-${i}`;
                const expanded = expandedFindings.has(fKey);

                return (
                    <div key={fKey}>
                        <button
                            onClick={() => toggleFinding(fKey)}
                            className="w-full flex items-center gap-2 px-4 py-1.5 hover:bg-surface-light/30 transition-colors text-left text-xs"
                        >
                            <span className="font-mono text-gray-400 flex-shrink-0">
                                {f.resource.kind.toLowerCase()}/{f.resource.name}
                            </span>
                            {f.resource.namespace && (
                                <span className="text-gray-500 flex-shrink-0">
                                    ns/{f.resource.namespace}
                                </span>
                            )}
                            <span className="flex-1" />
                            {expanded
                                ? <ChevronDownIcon className="h-3 w-3 text-gray-500 flex-shrink-0" />
                                : <ChevronRightIcon className="h-3 w-3 text-gray-500 flex-shrink-0" />
                            }
                        </button>

                        {expanded && (
                            <div className="px-4 pb-2 pt-1 ml-4 space-y-1.5">
                                <p className="text-xs text-gray-300">{f.description}</p>
                                {f.suggestedFix && (
                                    <div className="text-xs text-gray-400 bg-surface p-2 rounded border border-border">
                                        <span className="text-gray-500 font-medium">Fix: </span>
                                        {f.suggestedFix}
                                    </div>
                                )}
                                {f.details && Object.keys(f.details).length > 0 && (
                                    <div className="text-[10px] font-mono text-gray-500 space-y-0.5">
                                        {Object.entries(f.details).map(([k, v]) => (
                                            <div key={k}>
                                                <span className="text-gray-600">{k}:</span> {v}
                                            </div>
                                        ))}
                                    </div>
                                )}
                            </div>
                        )}
                    </div>
                );
            })}
        </>
    );

    return (
        <div className="h-full flex flex-col bg-background text-text">
            {/* Header */}
            <div className="flex-shrink-0 border-b border-border p-4 space-y-3">
                <div className="flex items-center justify-between">
                    <h2 className="text-lg font-semibold flex items-center gap-2">
                        <BugAntIcon className="h-5 w-5 text-amber-400" />
                        Issue Detector
                    </h2>
                    <div className="flex items-center gap-2">
                        <button
                            onClick={() => setShowRulesPanel(!showRulesPanel)}
                            className="px-3 py-1.5 text-xs bg-surface hover:bg-surface-light border border-border rounded transition-colors flex items-center gap-1.5"
                        >
                            <AdjustmentsHorizontalIcon className="h-3.5 w-3.5" />
                            Rules ({rules.length - disabledRules.size}/{rules.length})
                        </button>
                        {onClose && (
                            <button onClick={onClose} className="p-1 text-gray-400 hover:text-white hover:bg-white/10 rounded transition-colors">
                                <XMarkIcon className="h-5 w-5" />
                            </button>
                        )}
                    </div>
                </div>

                {/* Filters row */}
                <div className="flex flex-wrap items-center gap-3">
                    <div className="flex items-center gap-2">
                        <label className="text-xs text-gray-400">Namespace:</label>
                        <select
                            className="bg-surface border border-border rounded px-2 py-1 text-xs text-text"
                            value={selectedNamespaces.length === 0 ? '__all__' : selectedNamespaces[0]}
                            onChange={e => {
                                const val = e.target.value;
                                setSelectedNamespaces(val === '__all__' ? [] : [val]);
                            }}
                        >
                            <option value="__all__">All namespaces</option>
                            {(namespaces || []).map((ns: any) => (
                                <option key={typeof ns === 'string' ? ns : ns?.name} value={typeof ns === 'string' ? ns : ns?.name}>
                                    {typeof ns === 'string' ? ns : ns?.name}
                                </option>
                            ))}
                        </select>
                    </div>

                    <div className="flex items-center gap-2">
                        <label className="text-xs text-gray-400">Category:</label>
                        <select
                            className="bg-surface border border-border rounded px-2 py-1 text-xs text-text"
                            value={selectedCategories.length === 0 ? '__all__' : selectedCategories[0]}
                            onChange={e => {
                                const val = e.target.value;
                                setSelectedCategories(val === '__all__' ? [] : [val]);
                            }}
                        >
                            <option value="__all__">All categories</option>
                            {Object.entries(CATEGORY_LABELS).map(([k, v]) => (
                                <option key={k} value={k}>{v}</option>
                            ))}
                        </select>
                    </div>

                    <label className="flex items-center gap-1.5 text-xs text-gray-400 cursor-pointer">
                        <input
                            type="checkbox"
                            checked={clusterWide}
                            onChange={e => setClusterWide(e.target.checked)}
                            className="rounded border-border bg-surface"
                        />
                        Include cluster-wide
                    </label>

                    <button
                        onClick={handleScan}
                        disabled={scanning}
                        className="ml-auto px-4 py-1.5 bg-primary hover:bg-primary/80 disabled:opacity-50 disabled:cursor-not-allowed rounded text-sm font-medium text-white flex items-center gap-2 transition-colors"
                    >
                        {scanning ? (
                            <ArrowPathIcon className="h-4 w-4 animate-spin" />
                        ) : (
                            <MagnifyingGlassIcon className="h-4 w-4" />
                        )}
                        {scanning ? 'Scanning...' : 'Scan'}
                    </button>
                </div>
            </div>

            {/* Progress bar */}
            {scanning && progress && (
                <div className="flex-shrink-0 border-b border-border px-4 py-2">
                    <div className="flex items-center justify-between text-xs text-gray-400 mb-1">
                        <span>{progress.description}</span>
                        <span>{Math.round(progress.percent)}%</span>
                    </div>
                    <div className="w-full bg-surface rounded-full h-1.5">
                        <div
                            className="bg-primary h-1.5 rounded-full transition-all duration-300"
                            style={{ width: `${progress.percent}%` }}
                        />
                    </div>
                </div>
            )}

            {/* Error display */}
            {error && (
                <div className="flex-shrink-0 p-4 bg-red-900/20 border-b border-red-800/50 text-red-400 text-sm">
                    {error}
                </div>
            )}

            {/* Results area */}
            <div className="flex-1 min-h-0 flex overflow-hidden">
                {/* Main content */}
                <div className="flex-1 min-h-0 overflow-auto">
                    {!result && !scanning ? (
                        <div className="flex flex-col items-center justify-center h-full text-gray-500">
                            <BugAntIcon className="h-12 w-12 mb-3 opacity-50" />
                            <p className="text-sm">Run a scan to detect issues in your cluster</p>
                            <p className="text-xs mt-1 text-gray-600">Checks networking, workloads, storage, security, and config</p>
                        </div>
                    ) : result ? (
                        <div className="p-4 space-y-3">
                            {/* Summary bar */}
                            <div className="flex flex-wrap items-center gap-4 p-3 bg-surface rounded border border-border text-sm">
                                <div className="flex items-center gap-3">
                                    {(['critical', 'warning', 'info'] as const).map(sev => {
                                        const cfg = SEVERITY_CONFIG[sev];
                                        const count = severityCounts[sev];
                                        return (
                                            <span key={sev} className={`flex items-center gap-1 ${cfg.color}`}>
                                                <cfg.icon className="h-4 w-4" />
                                                {count} {cfg.label}
                                            </span>
                                        );
                                    })}
                                </div>
                                <span className="text-gray-500">|</span>
                                <span className="text-gray-400">{result.rulesRun} rules</span>
                                <span className="text-gray-500">|</span>
                                <span className="text-gray-400">{(result.durationMs / 1000).toFixed(1)}s</span>

                                {result.errors && result.errors.length > 0 && (
                                    <>
                                        <span className="text-gray-500">|</span>
                                        <Tooltip content={result.errors.join('\n')}>
                                            <span className="text-amber-400 cursor-help">{result.errors.length} warning(s)</span>
                                        </Tooltip>
                                    </>
                                )}
                            </div>

                            {/* Filter toolbar */}
                            <div className="flex flex-wrap items-center gap-3">
                                <div className="flex items-center gap-1.5">
                                    <label className="text-xs text-gray-400">Group by:</label>
                                    {(['severity', 'category', 'kind'] as const).map(g => (
                                        <button
                                            key={g}
                                            onClick={() => {
                                                setGroupBy(g);
                                                setExpandedGroups(new Set(grouped.map(gr => gr.key)));
                                            }}
                                            className={`px-2 py-0.5 rounded text-xs transition-colors ${
                                                groupBy === g ? 'bg-primary text-white' : 'bg-surface text-gray-300 hover:bg-surface-light'
                                            }`}
                                        >
                                            {g.charAt(0).toUpperCase() + g.slice(1)}
                                        </button>
                                    ))}
                                </div>

                                <div className="relative flex-1 min-w-[200px] max-w-xs">
                                    <MagnifyingGlassIcon className="absolute left-2 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-gray-500" />
                                    <input
                                        type="text"
                                        value={searchFilter}
                                        onChange={e => setSearchFilter(e.target.value)}
                                        placeholder="Filter findings..."
                                        className="w-full pl-7 pr-2 py-1 bg-surface border border-border rounded text-xs text-text placeholder-gray-500 focus:outline-none focus:ring-1 focus:ring-primary/50"
                                    />
                                </div>

                                <div className="flex items-center gap-1">
                                    {(['critical', 'warning', 'info'] as const).map(sev => {
                                        const cfg = SEVERITY_CONFIG[sev];
                                        const active = severityFilter.has(sev);
                                        return (
                                            <button
                                                key={sev}
                                                onClick={() => toggleSeverity(sev)}
                                                className={`px-2 py-0.5 rounded text-xs flex items-center gap-1 transition-colors border ${
                                                    active ? cfg.bgColor : 'bg-surface border-border text-gray-500'
                                                }`}
                                            >
                                                <cfg.icon className={`h-3 w-3 ${active ? cfg.color : ''}`} />
                                                {cfg.label}
                                            </button>
                                        );
                                    })}
                                </div>
                            </div>

                            {/* Findings list */}
                            {filteredFindings.length === 0 ? (
                                <div className="text-center py-8 text-gray-500 text-sm">
                                    {(result.findings?.length ?? 0) === 0
                                        ? 'No issues found - your cluster looks healthy!'
                                        : 'No findings match the current filters'}
                                </div>
                            ) : (
                                <div className="space-y-2">
                                    {grouped.map(group => (
                                        <div key={group.key} className="border border-border rounded overflow-hidden">
                                            <button
                                                onClick={() => toggleGroup(group.key)}
                                                className="w-full flex items-center gap-2 px-3 py-2 bg-surface hover:bg-surface-light transition-colors text-sm font-medium"
                                            >
                                                {expandedGroups.has(group.key)
                                                    ? <ChevronDownIcon className="h-4 w-4 text-gray-400" />
                                                    : <ChevronRightIcon className="h-4 w-4 text-gray-400" />
                                                }
                                                <span>{groupLabel(group.key)}</span>
                                                <span className="text-xs text-gray-500">({group.totalFindings})</span>
                                            </button>

                                            {expandedGroups.has(group.key) && (
                                                <div className="divide-y divide-border">
                                                    {group.ruleGroups.map(rg => {
                                                        const ruleKey = `${group.key}:${rg.ruleID}`;
                                                        const sevCfg = SEVERITY_CONFIG[rg.severity] || SEVERITY_CONFIG.info;
                                                        const ruleExpanded = expandedRules.has(ruleKey);

                                                        return (
                                                            <div key={ruleKey} className="bg-background">
                                                                {/* Rule header */}
                                                                <button
                                                                    onClick={() => toggleRuleGroup(ruleKey)}
                                                                    className="w-full flex items-center gap-3 px-4 py-2 hover:bg-surface-light/50 transition-colors text-left"
                                                                >
                                                                    {ruleExpanded
                                                                        ? <ChevronDownIcon className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" />
                                                                        : <ChevronRightIcon className="h-3.5 w-3.5 text-gray-400 flex-shrink-0" />
                                                                    }
                                                                    <sevCfg.icon className={`h-4 w-4 flex-shrink-0 ${sevCfg.color}`} />
                                                                    <span className="text-xs font-mono text-gray-500 flex-shrink-0">{rg.ruleID}</span>
                                                                    <span className="text-sm text-text flex-1 truncate">{rg.ruleName}</span>
                                                                    <span className="text-xs text-gray-500 flex-shrink-0">({rg.findings.length})</span>
                                                                </button>

                                                                {/* Resources under this rule */}
                                                                {ruleExpanded && (
                                                                    <div className="ml-6 border-l border-border/50">
                                                                        {rg.subGroups ? (
                                                                            // Render with sub-group headers
                                                                            rg.subGroups.map(sg => {
                                                                                const sgKey = `${ruleKey}:${sg.key}`;
                                                                                const sgExpanded = expandedSubGroups.has(sgKey);
                                                                                return (
                                                                                    <div key={sgKey}>
                                                                                        <button
                                                                                            onClick={() => toggleSubGroup(sgKey)}
                                                                                            className="w-full flex items-center gap-2 px-4 py-1.5 hover:bg-surface-light/30 transition-colors text-left"
                                                                                        >
                                                                                            {sgExpanded
                                                                                                ? <ChevronDownIcon className="h-3 w-3 text-gray-400 flex-shrink-0" />
                                                                                                : <ChevronRightIcon className="h-3 w-3 text-gray-400 flex-shrink-0" />
                                                                                            }
                                                                                            <span className="text-xs font-mono text-gray-300">{sg.key}</span>
                                                                                            <span className="text-[10px] text-gray-500">({sg.findings.length})</span>
                                                                                        </button>
                                                                                        {sgExpanded && (
                                                                                            <div className="ml-4">
                                                                                                {renderFindings(sg.findings, ruleKey)}
                                                                                            </div>
                                                                                        )}
                                                                                    </div>
                                                                                );
                                                                            })
                                                                        ) : (
                                                                            // Render flat list
                                                                            renderFindings(rg.findings, ruleKey)
                                                                        )}
                                                                    </div>
                                                                )}
                                                            </div>
                                                        );
                                                    })}
                                                </div>
                                            )}
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    ) : null}
                </div>

                {/* Rules side panel */}
                {showRulesPanel && (
                    <div className="w-80 flex-shrink-0 border-l border-border flex flex-col bg-background">
                        <div className="flex items-center justify-between p-3 border-b border-border">
                            <h3 className="text-sm font-medium">Rules</h3>
                            <div className="flex items-center gap-1">
                                <Tooltip content="Reload user rules from disk">
                                    <button onClick={reloadRules} className="p-1 text-gray-400 hover:text-white rounded transition-colors">
                                        <ArrowPathIcon className="h-4 w-4" />
                                    </button>
                                </Tooltip>
                                <Tooltip content="Open rules directory">
                                    <button onClick={openRulesDir} className="p-1 text-gray-400 hover:text-white rounded transition-colors">
                                        <FolderOpenIcon className="h-4 w-4" />
                                    </button>
                                </Tooltip>
                                <button onClick={() => setShowRulesPanel(false)} className="p-1 text-gray-400 hover:text-white rounded transition-colors">
                                    <XMarkIcon className="h-4 w-4" />
                                </button>
                            </div>
                        </div>

                        <div className="flex items-center justify-between px-3 py-2 border-b border-border">
                            <span className="text-xs text-gray-400">
                                {rules.length - disabledRules.size} of {rules.length} enabled
                            </span>
                            <button
                                onClick={() => {
                                    if (disabledRules.size === 0) {
                                        setDisabledRules(new Set(rules.map(r => r.id)));
                                    } else {
                                        setDisabledRules(new Set());
                                    }
                                }}
                                className="text-xs text-primary hover:text-primary/80 transition-colors"
                            >
                                {disabledRules.size === 0 ? 'Deselect all' : 'Select all'}
                            </button>
                        </div>

                        <div className="flex-1 min-h-0 overflow-auto p-2 space-y-1">
                            {rules.map(rule => {
                                const disabled = disabledRules.has(rule.id);
                                const sevCfg = SEVERITY_CONFIG[rule.severity] || SEVERITY_CONFIG.info;
                                return (
                                    <button
                                        key={rule.id}
                                        onClick={() => toggleRule(rule.id)}
                                        className={`w-full text-left px-3 py-2 rounded text-xs transition-colors border ${
                                            disabled ? 'border-border bg-surface/50 opacity-50' : 'border-border bg-surface hover:bg-surface-light'
                                        }`}
                                    >
                                        <div className="flex items-center gap-2">
                                            <div className={`w-4 h-4 rounded border flex items-center justify-center flex-shrink-0 ${
                                                disabled ? 'border-gray-600' : 'border-primary bg-primary/20'
                                            }`}>
                                                {!disabled && <CheckIcon className="h-3 w-3 text-primary" />}
                                            </div>
                                            <span className="font-mono text-gray-500">{rule.id}</span>
                                            <sevCfg.icon className={`h-3 w-3 flex-shrink-0 ${sevCfg.color}`} />
                                        </div>
                                        <div className="mt-1 ml-6 text-text">{rule.name}</div>
                                        {rule.description && (
                                            <div className="mt-0.5 ml-6 text-gray-500 text-[10px]">{rule.description}</div>
                                        )}
                                        {!rule.isBuiltin && (
                                            <div className="mt-0.5 ml-6">
                                                <span className="text-[10px] px-1 py-0.5 rounded bg-blue-900/30 text-blue-400 border border-blue-800/50">
                                                    user rule
                                                </span>
                                            </div>
                                        )}
                                    </button>
                                );
                            })}
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
