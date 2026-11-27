import type { SemanticsMap } from '~/utils/columnSemantics';
import { NODE_RECORD_FIELDS, recordOf, metricsRecordText, type NodeMetricData } from '~/utils/metricsTransform';

/** Conditions of a node as the Conditions column summarises them. */
export const getConditionsSummary = (node: any) => {
    const conditions = node.status?.conditions || [];
    const isUnschedulable = node.spec?.unschedulable === true;

    const issues: { type: string; severity: string }[] = [];

    // Check Ready condition
    const readyCondition = conditions.find((c: any) => c.type === 'Ready');
    const isReady = readyCondition?.status === 'True';

    if (!isReady) {
        issues.push({ type: 'NotReady', severity: 'error' });
    }

    if (isUnschedulable) {
        issues.push({ type: 'SchedulingDisabled', severity: 'warning' });
    }

    // Check pressure conditions (these are problems when True)
    const pressureConditions = ['MemoryPressure', 'DiskPressure', 'PIDPressure'];
    pressureConditions.forEach((condType: any) => {
        const cond = conditions.find((c: any) => c.type === condType);
        if (cond?.status === 'True') {
            issues.push({ type: condType, severity: 'error' });
        }
    });

    // Check NetworkUnavailable (problem when True)
    const networkCond = conditions.find((c: any) => c.type === 'NetworkUnavailable');
    if (networkCond?.status === 'True') {
        issues.push({ type: 'NetworkUnavailable', severity: 'error' });
    }

    return { isReady, isUnschedulable, issues };
};

/** The Conditions cell value: 'Ready' or the issue types joined with commas. */
export const conditionsText = (node: any): string => {
    const { issues } = getConditionsSummary(node);
    return issues.length === 0 ? 'Ready' : issues.map((i: any) => i.type).join(',');
};

const nodeRoleKeys = (node: any): string[] => {
    const labels = node.metadata?.labels || {};
    return Object.keys(labels)
        .filter((k: any) => k.startsWith('node-role.kubernetes.io/'))
        .map((k: any) => k.replace('node-role.kubernetes.io/', ''));
};

/** The Roles cell value: the role names joined with ', '. */
export const rolesText = (node: any): string => nodeRoleKeys(node).join(', ');

export const addressOf = (node: any, type: string): string =>
    (node.status?.addresses || []).find((a: any) => a.type === type)?.address || '';

/** Every taint as the tooltip lists it, one per line (display only). */
export const taintLines = (node: any): string =>
    (node.spec?.taints || []).map((t: any) => `${t.key}${t.value ? '=' + t.value : ''}:${t.effect}`).join('\n');

/** Where the metrics columns read their sample from (a store in the list, a fixture in the golden test). */
export type NodeMetricLookup = (item: any) => NodeMetricData | undefined;

/**
 * Sort/filter semantics of the Node list columns. The metrics columns read their sample through
 * `lookup`, so one definition serves the legacy list, the golden generator and the view overlay.
 */
export function createNodeSemantics(lookup: NodeMetricLookup) {
    return {
        name: { getValue: (item: any) => item.metadata?.name },
        cpu: {
            filterType: 'numeric' as const,
            getValue: (item: any) => lookup(item)?.cpuPercent ?? -1,
            getNumericValue: (item: any) => lookup(item)?.cpuPercent ?? NaN,
        },
        memory: {
            filterType: 'numeric' as const,
            getValue: (item: any) => lookup(item)?.memPercent ?? -1,
            getNumericValue: (item: any) => lookup(item)?.memPercent ?? NaN,
        },
        pods: { getValue: (item: any) => lookup(item)?.podPercent ?? -1 },
        conditions: { getValue: conditionsText },
        taints: { filterable: false, getValue: (item: any) => (item.spec?.taints || []).length },
        version: { getValue: (item: any) => item.status?.nodeInfo?.kubeletVersion },
        roles: { getValue: rolesText },
        internalIP: { getValue: (item: any) => addressOf(item, 'InternalIP') },
        externalIP: { getValue: (item: any) => addressOf(item, 'ExternalIP') },
        osImage: { getValue: (item: any) => item.status?.nodeInfo?.osImage || '' },
        kernelVersion: { getValue: (item: any) => item.status?.nodeInfo?.kernelVersion || '' },
        containerRuntime: { getValue: (item: any) => item.status?.nodeInfo?.containerRuntimeVersion || '' },
        architecture: { getValue: (item: any) => item.status?.nodeInfo?.architecture || '' },
        os: { getValue: (item: any) => item.status?.nodeInfo?.operatingSystem || '' },
        age: {
            getValue: (item: any) => item.metadata?.creationTimestamp,
            getNumericValue: (item: any) => {
                if (!item.metadata?.creationTimestamp) return NaN;
                return (Date.now() - new Date(item.metadata.creationTimestamp).getTime()) / 3600000;
            },
        },
        /** Not columns of the list: the view's display-only cells (the legacy renders read the object). */
        taintList: { getValue: taintLines, filterable: false },
        unschedulable: { getValue: (item: any) => (item.spec?.unschedulable === true ? 'true' : ''), filterable: false },
        metrics: {
            getValue: (item: any) => {
                const m = lookup(item);
                return m ? metricsRecordText(recordOf(NODE_RECORD_FIELDS, m)) : '';
            },
            filterable: false,
        },
    } satisfies SemanticsMap;
}

export const nodeSemantics = createNodeSemantics(() => undefined);
