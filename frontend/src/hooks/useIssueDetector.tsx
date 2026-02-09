// Type definitions for the Issue Detector feature.
// The hook logic now lives in ~/context/IssueDetectorContext.tsx.

export interface ScanProgress {
    phase: string;
    description: string;
    percent: number;
}

export interface ResourceRef {
    kind: string;
    name: string;
    namespace?: string;
}

export interface Finding {
    ruleID: string;
    ruleName: string;
    severity: string;
    category: string;
    resource: ResourceRef;
    description: string;
    suggestedFix?: string;
    details?: Record<string, string>;
    groupKey?: string;
}

export interface ScanResult {
    findings: Finding[];
    rulesRun: number;
    resourcesFetched: Record<string, number>;
    durationMs: number;
    errors?: string[];
}

export interface RuleInfo {
    id: string;
    name: string;
    description: string;
    severity: string;
    category: string;
    isBuiltin: boolean;
    requires: string[];
}

export type GroupBy = 'severity' | 'category' | 'kind';
