import React, { memo } from 'react';
import { getConditionsSummary } from './semantics';

export interface ConditionIssue {
    type: string;
    severity: string;
}

/** The condition badges (or "Ready") of the Conditions column. */
const ConditionBadges = memo(function ConditionBadges({ issues }: { issues: ConditionIssue[] }) {
    if (issues.length === 0) {
        return <span className="text-green-400">Ready</span>;
    }

    return (
        <div className="flex flex-wrap gap-1">
            {issues.map((issue: any, idx: number) => (
                <span
                    key={idx}
                    className={`px-1.5 py-0.5 text-xs rounded ${
                        issue.severity === 'error'
                            ? 'bg-red-500/20 text-red-400'
                            : 'bg-yellow-500/20 text-yellow-400'
                    }`}
                >
                    {issue.type}
                </span>
            ))}
        </div>
    );
});

// Conditions cell component
export const ConditionsCell = memo(function ConditionsCell({ node }: { node: any }) {
    const { issues } = getConditionsSummary(node);
    return <ConditionBadges issues={issues} />;
});

/** The same cell from the Conditions column value ("Ready" or the issue types joined with commas). */
export const ConditionsTextCell = memo(function ConditionsTextCell({ text }: { text: string | null | undefined }) {
    const issues: ConditionIssue[] = !text || text === 'Ready'
        ? []
        : text.split(',').map(type => ({ type, severity: type === 'SchedulingDisabled' ? 'warning' : 'error' }));
    return <ConditionBadges issues={issues} />;
});

/** Count with the taints in a tooltip; `lines` are the tooltip lines. */
const TaintBadge = memo(function TaintBadge({ count, lines }: { count: number; lines: string[] }) {
    if (count === 0) {
        return <span className="text-gray-500">-</span>;
    }

    return (
        <span className="relative group cursor-help text-gray-300 underline decoration-dotted">
            {count}
            <div className="absolute z-50 invisible opacity-0 group-hover:visible group-hover:opacity-100 transition-opacity delay-500 bottom-full left-1/2 -translate-x-1/2 mb-1 px-2 py-1 text-xs bg-background border border-border rounded shadow-lg whitespace-pre text-left">
                {lines.map((line: any, idx: number) => (
                    <div key={idx}>{line}</div>
                ))}
            </div>
        </span>
    );
});

// Taints cell component with tooltip
export const TaintsCell = memo(function TaintsCell({ node }: { node: any }) {
    const taints = node.spec?.taints || [];
    const lines = taints.map((t: any) => `${t.key}${t.value ? '=' + t.value : ''}:${t.effect}`);
    return <TaintBadge count={taints.length} lines={lines} />;
});

/** The same cell from the Taints count and the display-only taint list (one taint per line). */
export const TaintsTextCell = memo(function TaintsTextCell({ count, list }: { count: number | null | undefined; list: string | null | undefined }) {
    return <TaintBadge count={count ?? 0} lines={list ? list.split('\n') : []} />;
});
