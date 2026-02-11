/**
 * Utility functions for the Node Topology view.
 */

import { getPodStatus } from '~/utils/k8s-helpers';

/** Group pods by spec.nodeName, excluding Succeeded/completed pods */
export function groupPodsByNode(pods: any[]): Map<string, any[]> {
    const map = new Map<string, any[]>();
    for (const pod of pods) {
        const nodeName = pod.spec?.nodeName;
        if (!nodeName) continue;
        const status = pod.status?.phase;
        if (status === 'Succeeded') continue;
        const list = map.get(nodeName);
        if (list) {
            list.push(pod);
        } else {
            map.set(nodeName, [pod]);
        }
    }
    return map;
}

/** 20-color palette for namespace coloring */
const NAMESPACE_COLORS = [
    '#3b82f6', '#22c55e', '#f59e0b', '#ef4444', '#8b5cf6',
    '#06b6d4', '#ec4899', '#14b8a6', '#f97316', '#84cc16',
    '#a855f7', '#6366f1', '#d946ef', '#0ea5e9', '#10b981',
    '#e11d48', '#7c3aed', '#0891b2', '#ca8a04', '#dc2626',
];

/** Deterministic hash → color for a namespace string */
export function getNamespaceColor(namespace: string): string {
    let hash = 0;
    for (let i = 0; i < namespace.length; i++) {
        hash = ((hash << 5) - hash + namespace.charCodeAt(i)) | 0;
    }
    return NAMESPACE_COLORS[Math.abs(hash) % NAMESPACE_COLORS.length];
}

export type ZoomLevel = 'far' | 'medium' | 'close';
export type ColorMode = 'status' | 'resource';
export type EvictionCategory = 'reschedulable' | 'killable' | 'daemon';

/** Map continuous zoom value to discrete level */
export function getZoomLevel(zoom: number): ZoomLevel {
    if (zoom < 0.4) return 'far';
    if (zoom < 0.8) return 'medium';
    return 'close';
}

/** Compute simple grid positions for N nodes */
export function computeGridPositions(
    nodeCount: number,
    nodeWidth: number = 300,
    nodeHeight: number = 220,
    hSpacing: number = 400,
    vSpacing: number = 350,
): { x: number; y: number }[] {
    const cols = Math.max(1, Math.ceil(Math.sqrt(nodeCount)));
    const positions: { x: number; y: number }[] = [];
    for (let i = 0; i < nodeCount; i++) {
        const col = i % cols;
        const row = Math.floor(i / cols);
        positions.push({ x: col * hSpacing, y: row * vSpacing });
    }
    return positions;
}

/** Node card layout constants */
const NODE_WIDTH = 280;
const NODE_PADDING = 24; // px-3 = 12px each side
const GRID_GAP = 4; // gap-1

/** Pod square size (consistent across medium and close zoom) */
const POD_SQUARE_SIZE = 16;
/** Max pod grid rows — fewer at close zoom to make room for namespace labels */
const MAX_POD_ROWS_MEDIUM = 8;
const MAX_POD_ROWS_CLOSE = 6;

/** Get pod square size (same at medium and close zoom) */
export function getPodSquareSize(_zoomLevel: ZoomLevel): number {
    return POD_SQUARE_SIZE;
}

/** Get the number of columns that fit in the pod grid */
export function getPodGridCols(): number {
    return Math.floor((NODE_WIDTH - NODE_PADDING + GRID_GAP) / (POD_SQUARE_SIZE + GRID_GAP));
}

/** Max visible pods for a zoom level */
export function getMaxVisiblePods(zoomLevel: ZoomLevel): number {
    const maxRows = zoomLevel === 'close' ? MAX_POD_ROWS_CLOSE : MAX_POD_ROWS_MEDIUM;
    return getPodGridCols() * maxRows;
}

/** Compute the pod grid height for a node card */
export function computePodGridHeight(podCount: number, zoomLevel: ZoomLevel): number {
    if (zoomLevel === 'far' || podCount === 0) return 0;
    const cols = getPodGridCols();
    const maxRows = zoomLevel === 'close' ? MAX_POD_ROWS_CLOSE : MAX_POD_ROWS_MEDIUM;
    const maxPods = cols * maxRows;
    const rows = Math.ceil(Math.min(podCount, maxPods) / cols);
    return rows * (POD_SQUARE_SIZE + GRID_GAP) + 16;
}

/** Get hex color for a pod status */
export function getPodStatusBgColor(status: string): string {
    switch (status) {
        case 'Running':
            return '#22c55e';
        case 'Pending':
        case 'ContainerCreating':
        case 'Init:Running':
        case 'Init:Waiting':
        case 'PodInitializing':
            return '#f59e0b';
        case 'Terminating':
            return '#6b7280';
        case 'CrashLoopBackOff':
        case 'ImagePullBackOff':
        case 'ErrImagePull':
        case 'Init:CrashLoopBackOff':
        case 'Init:ImagePullBackOff':
        case 'Init:ErrImagePull':
        case 'Unknown':
            return '#f97316';
        case 'Failed':
        case 'Init:Error':
            return '#ef4444';
        default:
            return '#6b7280';
    }
}

// ---------------------------------------------------------------------------
// Resource-based coloring
// ---------------------------------------------------------------------------

/** Parse k8s CPU quantity to millicores. Returns 0 for malformed input. */
function parseCpuQuantity(q: string | undefined): number {
    if (!q) return 0;
    let v: number;
    if (q.endsWith('n')) v = parseInt(q, 10) / 1e6;
    else if (q.endsWith('u')) v = parseInt(q, 10) / 1e3;
    else if (q.endsWith('m')) v = parseInt(q, 10);
    else v = parseFloat(q) * 1000;
    return Number.isFinite(v) ? v : 0;
}

/** Parse k8s memory quantity to bytes. Returns 0 for malformed input. */
function parseMemoryQuantity(q: string | undefined): number {
    if (!q) return 0;
    const units: [string, number][] = [
        ['Ti', 1024 ** 4], ['Gi', 1024 ** 3], ['Mi', 1024 ** 2], ['Ki', 1024],
        ['T', 1e12], ['G', 1e9], ['M', 1e6], ['K', 1e3],
    ];
    for (const [suffix, mult] of units) {
        if (q.endsWith(suffix)) {
            const v = parseFloat(q.slice(0, -suffix.length)) * mult;
            return Number.isFinite(v) ? v : 0;
        }
    }
    const v = parseFloat(q);
    return Number.isFinite(v) ? v : 0;
}

/** Sum CPU and memory requests across all containers in a pod */
export function getPodResourceRequests(pod: any): { cpuMillis: number; memBytes: number } {
    let cpuMillis = 0;
    let memBytes = 0;
    for (const c of pod.spec?.containers || []) {
        cpuMillis += parseCpuQuantity(c.resources?.requests?.cpu);
        memBytes += parseMemoryQuantity(c.resources?.requests?.memory);
    }
    return { cpuMillis, memBytes };
}

/** Pod metrics map: keyed by "namespace/name" */
export type PodMetricsMap = Record<string, { cpuCommitted: number; memCommitted: number }>;

/**
 * Get committed resources for a pod: max(usage, request).
 * Uses live metrics when available, falls back to spec requests.
 */
export function getPodCommitted(pod: any, podMetrics?: PodMetricsMap): { cpuMillis: number; memBytes: number } {
    if (podMetrics) {
        const key = `${pod.metadata?.namespace || ''}/${pod.metadata?.name || ''}`;
        const m = podMetrics[key];
        if (m) return { cpuMillis: m.cpuCommitted, memBytes: m.memCommitted };
    }
    return getPodResourceRequests(pod);
}

/** Max resource values across a set of pods, used for relative coloring */
export interface ResourceMaxes {
    maxCpu: number;  // millicores
    maxMem: number;  // bytes
}

/** Compute the max committed resources across all pods */
export function computeResourceMaxes(pods: any[], podMetrics?: PodMetricsMap): ResourceMaxes {
    let maxCpu = 0;
    let maxMem = 0;
    for (const pod of pods) {
        const { cpuMillis, memBytes } = getPodCommitted(pod, podMetrics);
        if (cpuMillis > maxCpu) maxCpu = cpuMillis;
        if (memBytes > maxMem) maxMem = memBytes;
    }
    return { maxCpu, maxMem };
}

/**
 * Get pod color based on relative resource intensity (committed values).
 * 5-bucket heatmap: gray (none) → blue (low) → green (moderate) → amber (high) → red (top).
 */
export function getPodResourceColor(pod: any, maxes: ResourceMaxes, podMetrics?: PodMetricsMap): string {
    const { cpuMillis, memBytes } = getPodCommitted(pod, podMetrics);
    if (cpuMillis === 0 && memBytes === 0) return '#6b7280'; // gray — no requests

    const cpuRatio = maxes.maxCpu > 0 ? cpuMillis / maxes.maxCpu : 0;
    const memRatio = maxes.maxMem > 0 ? memBytes / maxes.maxMem : 0;
    const intensity = Math.max(cpuRatio, memRatio);

    if (intensity > 0.75) return '#ef4444'; // red-500    — top consumer
    if (intensity > 0.50) return '#f59e0b'; // amber-500  — high
    if (intensity > 0.25) return '#22c55e'; // green-500  — moderate
    return '#3b82f6';                       // blue-500   — low
}

/** Get the color for a pod based on the active color mode */
export function getPodSquareColor(pod: any, colorMode: ColorMode, maxes?: ResourceMaxes, podMetrics?: PodMetricsMap): string {
    if (colorMode === 'resource') return getPodResourceColor(pod, maxes || { maxCpu: 0, maxMem: 0 }, podMetrics);
    return getPodStatusBgColor(getPodStatus(pod));
}

// ---------------------------------------------------------------------------
// Pod sorting by color mode
// ---------------------------------------------------------------------------

/** Resource intensity score (higher = bigger consumer), uses committed values */
export function getPodResourceIntensity(pod: any, maxes: ResourceMaxes, podMetrics?: PodMetricsMap): number {
    const { cpuMillis, memBytes } = getPodCommitted(pod, podMetrics);
    if (cpuMillis === 0 && memBytes === 0) return -1; // no committed → sort last
    const cpuRatio = maxes.maxCpu > 0 ? cpuMillis / maxes.maxCpu : 0;
    const memRatio = maxes.maxMem > 0 ? memBytes / maxes.maxMem : 0;
    return Math.max(cpuRatio, memRatio);
}

/** Status sort priority — lower number = shown first */
const STATUS_SORT_PRIORITY: Record<string, number> = {
    'Failed': 0,
    'Init:Error': 0,
    'CrashLoopBackOff': 1,
    'Init:CrashLoopBackOff': 1,
    'ImagePullBackOff': 1,
    'ErrImagePull': 1,
    'Init:ImagePullBackOff': 1,
    'Init:ErrImagePull': 1,
    'Unknown': 1,
    'Pending': 2,
    'ContainerCreating': 2,
    'Init:Running': 2,
    'Init:Waiting': 2,
    'PodInitializing': 2,
    'Terminating': 3,
    'Running': 4,
};

/** Secondary sort: namespace then name */
function secondaryCompare(a: any, b: any): number {
    const nsA = a.metadata?.namespace || '';
    const nsB = b.metadata?.namespace || '';
    if (nsA !== nsB) return nsA.localeCompare(nsB);
    return (a.metadata?.name || '').localeCompare(b.metadata?.name || '');
}

/** Sort pods: primary by color mode, secondary by namespace then name */
export function sortPods(pods: any[], colorMode: ColorMode, maxes: ResourceMaxes, podMetrics?: PodMetricsMap): any[] {
    const sorted = pods.slice();
    if (colorMode === 'resource') {
        sorted.sort((a, b) => {
            const d = getPodResourceIntensity(b, maxes, podMetrics) - getPodResourceIntensity(a, maxes, podMetrics);
            return d !== 0 ? d : secondaryCompare(a, b);
        });
    } else {
        sorted.sort((a, b) => {
            const pa = STATUS_SORT_PRIORITY[getPodStatus(a)] ?? 3;
            const pb = STATUS_SORT_PRIORITY[getPodStatus(b)] ?? 3;
            if (pa !== pb) return pa - pb;
            return secondaryCompare(a, b);
        });
    }
    return sorted;
}

/** Format millicores for display */
export function formatCpuMillis(m: number): string {
    if (m === 0) return '-';
    if (m >= 1000) return `${(m / 1000).toFixed(1)}`;
    return `${Math.round(m)}m`;
}

/** Format bytes for display */
export function formatMemBytes(b: number): string {
    if (b === 0) return '-';
    if (b >= 1024 ** 3) return `${(b / 1024 ** 3).toFixed(1)}Gi`;
    if (b >= 1024 ** 2) return `${(b / 1024 ** 2).toFixed(0)}Mi`;
    if (b >= 1024) return `${(b / 1024).toFixed(0)}Ki`;
    return `${b}B`;
}
