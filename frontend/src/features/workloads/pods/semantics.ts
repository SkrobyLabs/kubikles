import type { SemanticsMap } from '~/utils/columnSemantics';
import { getPodStatus, getPodStatusPriority, getPodController } from '~/utils/k8s-helpers';
import { controllerRefText } from '../workloadSemantics';
import { POD_RECORD_FIELDS, recordOf, metricsRecordText, type PodMetricData } from '~/utils/metricsTransform';

/** Key of a pod in the metrics map. */
export const podMetricKey = (item: any): string => `${item.metadata?.namespace}/${item.metadata?.name}`;

/** Where the metrics columns read their sample from (a store in the list, a fixture in the golden test). */
export type PodMetricLookup = (item: any) => PodMetricData | undefined;

/** Total restart count over the container statuses. */
export const podRestarts = (item: any): number =>
    item.status?.containerStatuses?.reduce((acc: number, curr: any) => acc + curr.restartCount, 0) || 0;

/** Pod containers' images, one per line (display only; the legacy Image cell reads spec.containers). */
export const podImageLines = (item: any): string => (item.spec?.containers || []).map((c: any) => c.image).join('\n');

/** The controller reference as one JSON string, '' without one (display only: the link needs the whole reference). */
export const podControllerRefText = controllerRefText;

export interface ContainerStateCell {
    /** container name */
    n: string;
    /** first key of status.state: running, waiting, terminated or '' */
    s: string;
    /** waiting reason */
    w: string;
    /** terminated exit code, null otherwise */
    e: number | null;
}

/** The container status squares of the Containers column as data (display only). */
export const podContainerStatesText = (item: any): string => {
    const cells: ContainerStateCell[] = (item.status?.containerStatuses || []).map((status: any) => ({
        n: status.name,
        s: Object.keys(status.state || {})[0] || '',
        w: status.state?.waiting?.reason || '',
        e: status.state?.terminated ? status.state.terminated.exitCode : null,
    }));
    return JSON.stringify(cells);
};

/**
 * Sort/filter semantics of the Pod list columns. The metrics columns read their sample through
 * `lookup`, so the same code serves the legacy list (a metrics store), the golden generator and
 * the view-mode overlay.
 */
export function createPodSemantics(lookup: PodMetricLookup) {
    return {
        name: { getValue: (item: any) => item.metadata?.name },
        namespace: { getValue: (item: any) => item.metadata?.namespace },
        cpu: {
            filterType: 'numeric' as const,
            getValue: (item: any) => lookup(item)?.cpuCommittedPercent ?? -1,
            getNumericValue: (item: any) => lookup(item)?.cpuPercent ?? NaN,
        },
        memory: {
            filterType: 'numeric' as const,
            getValue: (item: any) => lookup(item)?.memCommittedPercent ?? -1,
            getNumericValue: (item: any) => lookup(item)?.memPercent ?? NaN,
        },
        containers: { filterable: false, getValue: (item: any) => getPodStatusPriority(getPodStatus(item)) },
        status: {
            getValue: (item: any) => getPodStatusPriority(getPodStatus(item)),
            getFilterValue: (item: any) => getPodStatus(item),
        },
        restarts: { filterType: 'numeric' as const, getValue: podRestarts, getNumericValue: podRestarts },
        age: {
            getValue: (item: any) => item.metadata?.creationTimestamp,
            getNumericValue: (item: any) => {
                if (!item.metadata?.creationTimestamp) return NaN;
                return (Date.now() - new Date(item.metadata.creationTimestamp).getTime()) / 3600000;
            },
        },
        controlledBy: { getValue: (item: any) => getPodController(item)?.kind || '' },
        qos: { getValue: (item: any) => item.status?.qosClass || '' },
        node: { getValue: (item: any) => item.spec?.nodeName || '' },
        podIP: { getValue: (item: any) => item.status?.podIP || '' },
        hostIP: { getValue: (item: any) => item.status?.hostIP || '' },
        serviceAccount: { getValue: (item: any) => item.spec?.serviceAccountName || 'default' },
        priorityClass: { getValue: (item: any) => item.spec?.priorityClassName || '' },
        image: { getValue: (item: any) => item.spec?.containers?.[0]?.image || '' },
        /**
         * Not columns of the list: the view's display-only cells. The legacy renders read the
         * object (status text, container squares, controller link, image list, metrics sample).
         */
        statusText: { getValue: (item: any) => getPodStatus(item), filterable: false },
        phase: { getValue: (item: any) => item.status?.phase || '', filterable: false },
        containerStates: { getValue: podContainerStatesText, filterable: false },
        controllerRef: { getValue: podControllerRefText, filterable: false },
        imageList: { getValue: podImageLines, filterable: false },
        metrics: {
            getValue: (item: any) => {
                const m = lookup(item);
                return m ? metricsRecordText(recordOf(POD_RECORD_FIELDS, m)) : '';
            },
            filterable: false,
        },
    } satisfies SemanticsMap;
}

const noMetrics: PodMetricLookup = () => undefined;

/** The semantics without a metrics source: what the list shows before the first sample. */
export const podSemantics = createPodSemantics(noMetrics);
