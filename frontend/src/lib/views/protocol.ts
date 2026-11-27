// TypeScript mirror of the Go view protocol (pkg/rows/types.go). These are wire shapes: the
// fixtures in __fixtures__ are captured from the real server and protocol.test.ts checks them.

export interface SortSpec {
    key: string;
    dir: 'asc' | 'desc' | '';
}

/** One condition of a column filter: a bare pattern (regex) or an operator and number (numeric). */
export type FilterConditionWire = string | { operator?: string; value?: number };

export interface ColumnFilterSpec {
    col: string;
    type: 'select' | 'regex' | 'numeric';
    values?: string[];
    conditions?: FilterConditionWire[];
    logic?: 'and' | 'or';
    pattern?: string;
    operator?: string;
    value?: number;
}

export interface ViewSpec {
    viewId: string;
    clientToken: string;
    clientId?: string;
    type: string;
    namespaces: string[];
    search: string;
    columnFilters: ColumnFilterSpec[];
    sort: SortSpec;
    columns: string[];
    locale: string;
    hide: string[];
    /** Custom resource descriptor for "crd:group/version/resource" types. */
    crd?: CrdDescriptor;
    /** Event kinds the view reports in notices besides counts: "pods" (pod views) reports restarts, vanished and new pods. */
    notifyOn?: string[];
}

export interface CrdDescriptor {
    group: string;
    version: string;
    resource: string;
    namespaced: boolean;
}

/** Row schema of a dynamic type, delivered with the open result (same shape as a generated one). */
export interface DynamicRowSchema {
    type: string;
    namespaced: boolean;
    schemaVersion: string;
    migrated: true;
    columns: Array<{
        key: string;
        kind: 'string' | 'number' | 'time' | 'bool';
        filterType: 'select' | 'regex' | 'numeric' | 'none';
        predefined: string[];
        volatile: boolean;
        /** Hidden in the column picker until the user shows it (printer column priority above 0). */
        hidden?: boolean;
        label?: string;
    }>;
    search: Array<{ name: string; aliases: string[] }>;
    presets: string[];
}

export interface ViewPatch {
    namespaces?: string[];
    search?: string;
    columnFilters?: ColumnFilterSpec[];
    sort?: SortSpec;
    columns?: string[];
    locale?: string;
    hide?: string[];
    notifyOn?: string[];
}

export type ViewPhase = 'loading' | 'refreshing' | 'complete' | 'incomplete';

/** State of one overlay source (pod, node or namespace metrics) of a view. */
export interface OverlayInfo {
    /** Whether the source could answer: null until its first answer, false when it cannot (metrics unavailable). */
    available: boolean | null;
}

export type OverlayStates = Record<string, OverlayInfo>;

/** Overlay source names (pkg/rows/overlay_metrics.go). */
export const OVERLAY_POD_METRICS = 'podmetrics';
export const OVERLAY_NODE_METRICS = 'nodemetrics';
export const OVERLAY_NAMESPACE_METRICS = 'namespacemetrics';

export interface ViewState {
    version: number;
    total: number;
    filtered: number;
    phase: ViewPhase;
    loaded: number;
    expected: number;
    error?: string;
    forbidden?: boolean;
    degraded?: boolean;
    filterable?: Record<string, boolean>;
    extra?: Record<string, unknown>;
    /** Availability of each overlay source the type has; absent for types without overlays. */
    overlays?: OverlayStates;
}

export interface ViewOpenResult {
    viewId: string;
    columns: string[];
    schemaVersion: string;
    state: ViewState;
    filterable: Record<string, boolean>;
    schema?: DynamicRowSchema;
}

/** One projected row as it travels: cells align with the open result's columns. */
export interface RowWire {
    seq: number;
    ns: string;
    name: string;
    ts: string;
    del: boolean;
    c: unknown[];
}

export interface ViewWindow {
    version: number;
    total: number;
    filtered: number;
    offset: number;
    order: string[];
    rows: Record<string, RowWire>;
    state: ViewState;
}

export interface RowRef {
    uid: string;
    ns: string;
    name: string;
}

export interface Facet {
    column: string;
    values: string[];
    counts: Record<string, number>;
    suppressed: boolean;
}

export interface SelectionAck {
    selected: number;
    stale: string[];
}

/** A pod event reported by a view opened with notifyOn "pods" (the Go Transition). */
export interface PodTransition {
    kind: 'restart' | 'deleted' | 'added';
    uid: string;
    namespace: string;
    name: string;
    /** "{kind}/{name}" of the first owner reference, absent without one. */
    owner?: string;
    /** Restart only: the pod's total restart count and the containers whose count rose. */
    total?: number;
    containers?: Array<{ name: string; count: number }>;
}

export interface ViewNotice {
    v: string;
    version: number;
    total: number;
    filtered: number;
    phase: ViewPhase;
    loaded: number;
    expected: number;
    closed?: string;
    selectionStale?: string[];
    windowChanged?: boolean;
    overlays?: OverlayStates;
    /** Pod events since the previous notice (views opened with notifyOn "pods"). */
    transitions?: PodTransition[];
}

export interface ObjectRef {
    type: string;
    namespace: string;
    name: string;
    uid?: string;
}

export interface ViewsInfo {
    enabled: boolean;
    types: string[];
    /** The backend builds row specs for "crd:" types at open time. */
    dynamic?: boolean;
}

/** First line of the NDJSON refs stream. */
export interface StreamHeader {
    version: number;
    total: number;
    filtered: number;
    columns?: string[];
}

export function isViewWindow(v: unknown): v is ViewWindow {
    if (!v || typeof v !== 'object') return false;
    const w = v as Record<string, unknown>;
    return typeof w.version === 'number' && typeof w.total === 'number' && typeof w.filtered === 'number' &&
        typeof w.offset === 'number' && Array.isArray(w.order) && !!w.rows && typeof w.rows === 'object' &&
        !!w.state && typeof w.state === 'object';
}

/** The server cannot find the view (410): re-open it from the retained spec. */
export class ViewGoneError extends Error {
    constructor(message = 'view not found') {
        super(message);
        this.name = 'ViewGoneError';
    }
}

/** The list changed since the caller read its row indexes (409): refresh and ask again. */
export class ViewStaleError extends Error {
    constructor(message = 'view changed: refresh and retry') {
        super(message);
        this.name = 'ViewStaleError';
    }
}

/** True for errors that mean the row indexes the caller holds no longer match the list. */
export function isViewStale(e: unknown): boolean {
    if (e instanceof ViewStaleError) return true;
    const status = (e as { status?: number } | null)?.status;
    if (status === 409) return true;
    const msg = typeof e === 'string' ? e : (e as Error | null)?.message ?? '';
    return /view changed/i.test(msg);
}

/** True for errors that mean the view no longer exists on the server. */
export function isViewGone(e: unknown): boolean {
    if (e instanceof ViewGoneError) return true;
    const status = (e as { status?: number } | null)?.status;
    if (status === 410) return true;
    const msg = typeof e === 'string' ? e : (e as Error | null)?.message ?? '';
    return /view not found/i.test(msg);
}
