import React, { createContext, useContext, useState, useCallback, useEffect, useRef, useMemo } from 'react';
import { RunIssueScan, ListIssueRules, ReloadIssueRules, GetIssueRulesDir, OpenIssueRulesDir } from 'wailsjs/go/main/App';
import { EventsOn, EventsOff } from 'wailsjs/runtime/runtime';
import { useK8s } from './K8sContext';
import type { ScanProgress, ScanResult, RuleInfo, Finding, GroupBy } from '~/hooks/useIssueDetector';

// ===========================
// Context Type
// ===========================

interface IssueDetectorContextType {
    // Scan state
    scanning: boolean;
    progress: ScanProgress | null;
    result: ScanResult | null;
    rules: RuleInfo[];
    error: string | null;
    rulesDir: string;

    // Scan actions
    runScan: (namespaces: string[], categories: string[], disabledRules: string[], clusterWide: boolean) => Promise<void>;
    reloadRules: () => Promise<void>;
    openRulesDir: () => Promise<void>;

    // UI filter state
    selectedNamespaces: string[];
    setSelectedNamespaces: React.Dispatch<React.SetStateAction<string[]>>;
    selectedCategories: string[];
    setSelectedCategories: React.Dispatch<React.SetStateAction<string[]>>;
    clusterWide: boolean;
    setClusterWide: React.Dispatch<React.SetStateAction<boolean>>;
    disabledRules: Set<string>;
    setDisabledRules: React.Dispatch<React.SetStateAction<Set<string>>>;
    groupBy: GroupBy;
    setGroupBy: React.Dispatch<React.SetStateAction<GroupBy>>;
    searchFilter: string;
    setSearchFilter: React.Dispatch<React.SetStateAction<string>>;
    severityFilter: Set<string>;
    setSeverityFilter: React.Dispatch<React.SetStateAction<Set<string>>>;
    expandedFindings: Set<string>;
    setExpandedFindings: React.Dispatch<React.SetStateAction<Set<string>>>;
    expandedGroups: Set<string>;
    setExpandedGroups: React.Dispatch<React.SetStateAction<Set<string>>>;
    expandedRules: Set<string>;
    setExpandedRules: React.Dispatch<React.SetStateAction<Set<string>>>;
    expandedSubGroups: Set<string>;
    setExpandedSubGroups: React.Dispatch<React.SetStateAction<Set<string>>>;
    showRulesPanel: boolean;
    setShowRulesPanel: React.Dispatch<React.SetStateAction<boolean>>;
}

const IssueDetectorContext = createContext<IssueDetectorContextType | null>(null);

// ===========================
// Provider
// ===========================

export function IssueDetectorProvider({ children }: { children: React.ReactNode }) {
    const { currentContext } = useK8s();

    // --- Scan state ---
    const [scanning, setScanning] = useState(false);
    const [progress, setProgress] = useState<ScanProgress | null>(null);
    const [result, setResult] = useState<ScanResult | null>(null);
    const [rules, setRules] = useState<RuleInfo[]>([]);
    const [error, setError] = useState<string | null>(null);
    const [rulesDir, setRulesDir] = useState('');
    const rulesLoaded = useRef(false);

    // --- UI filter state ---
    const [selectedNamespaces, setSelectedNamespaces] = useState<string[]>([]);
    const [selectedCategories, setSelectedCategories] = useState<string[]>([]);
    const [clusterWide, setClusterWide] = useState(true);
    const [disabledRules, setDisabledRules] = useState<Set<string>>(new Set());
    const [groupBy, setGroupBy] = useState<GroupBy>('severity');
    const [searchFilter, setSearchFilter] = useState('');
    const [severityFilter, setSeverityFilter] = useState<Set<string>>(new Set(['critical', 'warning', 'info']));
    const [expandedFindings, setExpandedFindings] = useState<Set<string>>(new Set());
    const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
    const [expandedRules, setExpandedRules] = useState<Set<string>>(new Set());
    const [expandedSubGroups, setExpandedSubGroups] = useState<Set<string>>(new Set());
    const [showRulesPanel, setShowRulesPanel] = useState(false);

    // --- Progress event listener (always active) ---
    useEffect(() => {
        const cleanup = EventsOn('issuedetector:progress', (data: ScanProgress) => {
            setProgress(data);
        });
        return () => {
            if (typeof cleanup === 'function') cleanup();
            else EventsOff('issuedetector:progress');
        };
    }, []);

    // --- Load rules ---
    const loadRules = useCallback(async () => {
        try {
            const [ruleList, dir] = await Promise.all([
                ListIssueRules(),
                GetIssueRulesDir(),
            ]);
            setRules(ruleList || []);
            setRulesDir(dir || '');
        } catch (err: any) {
            console.error('[IssueDetector] Failed to load rules:', err);
        }
    }, []);

    useEffect(() => {
        if (!rulesLoaded.current) {
            rulesLoaded.current = true;
            loadRules();
        }
    }, [loadRules]);

    // --- Context switch: clear cluster-specific state, reload rules ---
    const prevContext = useRef(currentContext);
    useEffect(() => {
        if (prevContext.current === currentContext) return;
        prevContext.current = currentContext;

        // Clear cluster-specific state
        setResult(null);
        setError(null);
        setProgress(null);
        setScanning(false);
        setExpandedFindings(new Set());
        setExpandedGroups(new Set());
        setExpandedRules(new Set());
        setExpandedSubGroups(new Set());
        setSearchFilter('');
        setSelectedNamespaces([]);

        // Reload rules (different cluster may have different custom rules)
        rulesLoaded.current = false;
        loadRules().then(() => { rulesLoaded.current = true; });
    }, [currentContext, loadRules]);

    // --- Scan actions ---
    const runScan = useCallback(async (
        namespaces: string[],
        categories: string[],
        disabled: string[],
        includeClusterWide: boolean,
    ) => {
        setScanning(true);
        setError(null);
        setProgress({ phase: 'starting', description: 'Starting scan...', percent: 0 });

        try {
            const scanResult = await RunIssueScan(namespaces, categories, disabled, includeClusterWide);
            setResult(scanResult);
        } catch (err: any) {
            setError(err?.message || String(err));
            setResult(null);
        } finally {
            setScanning(false);
            setProgress(null);
        }
    }, []);

    const reloadRules = useCallback(async () => {
        try {
            const ruleList = await ReloadIssueRules();
            setRules(ruleList || []);
        } catch (err: any) {
            console.error('[IssueDetector] Failed to reload rules:', err);
        }
    }, []);

    const openRulesDir = useCallback(async () => {
        try {
            await OpenIssueRulesDir();
        } catch (err: any) {
            console.error('[IssueDetector] Failed to open rules dir:', err);
        }
    }, []);

    // --- Memoized context value ---
    const value = useMemo(() => ({
        scanning,
        progress,
        result,
        rules,
        error,
        rulesDir,
        runScan,
        reloadRules,
        openRulesDir,
        selectedNamespaces,
        setSelectedNamespaces,
        selectedCategories,
        setSelectedCategories,
        clusterWide,
        setClusterWide,
        disabledRules,
        setDisabledRules,
        groupBy,
        setGroupBy,
        searchFilter,
        setSearchFilter,
        severityFilter,
        setSeverityFilter,
        expandedFindings,
        setExpandedFindings,
        expandedGroups,
        setExpandedGroups,
        expandedRules,
        setExpandedRules,
        expandedSubGroups,
        setExpandedSubGroups,
        showRulesPanel,
        setShowRulesPanel,
    }), [
        scanning, progress, result, rules, error, rulesDir,
        runScan, reloadRules, openRulesDir,
        selectedNamespaces, selectedCategories, clusterWide, disabledRules,
        groupBy, searchFilter, severityFilter,
        expandedFindings, expandedGroups, expandedRules, expandedSubGroups,
        showRulesPanel,
    ]);

    return (
        <IssueDetectorContext.Provider value={value}>
            {children}
        </IssueDetectorContext.Provider>
    );
}

// ===========================
// Hook
// ===========================

export function useIssueDetector(): IssueDetectorContextType {
    const ctx = useContext(IssueDetectorContext);
    if (!ctx) throw new Error('useIssueDetector must be used within IssueDetectorProvider');
    return ctx;
}
