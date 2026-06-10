import React from 'react';

// Column definitions, width constants, and the data-driven column-width
// calculation used by ResourceList. Extracted as pure logic so it can be unit-tested.

export const MIN_COLUMN_WIDTHS: Record<string, number> = {
    _selection: 44,
    name: 150,
    namespace: 120,
    message: 200,
    cpu: 100,
    memory: 100,
    containers: 120,
    restarts: 80,
    age: 80,
    controlledBy: 120,
    pods: 100,
    ready: 80,
    desired: 80,
    current: 80,
    available: 80,
    condition: 120,
    conditions: 120,
    completions: 80,
    lastRun: 100,
    nextRun: 100,
    suspend: 80,
    schedule: 100,
    taints: 80,
    version: 80,
    count: 80,
    type: 80,
    last: 80,
    involvedObject: 200,
    actions: 50,
    status: 100,
    cluster: 100,
    clusterIP: 120,
    externalIP: 120,
    ports: 150,
    selector: 150,
    labels: 150,
    node: 120,
    ip: 100,
    image: 200,
    reason: 120,
    source: 100,
};

// Default column widths for common columns (fallback if not calculated)
export const DEFAULT_COLUMN_WIDTHS: Record<string, number> = {
    cpu: 100,
    memory: 100,
    containers: 150,
    restarts: 140,
    age: 100,
    controlledBy: 150,
    pods: 100,
    ready: 100,
    desired: 100,
    current: 100,
    available: 100,
    condition: 150,
    conditions: 150,
    completions: 100,
    lastRun: 125,
    nextRun: 125,
    suspend: 100,
    schedule: 125,
    taints: 85,
    version: 85,
    count: 85,
    type: 100,
    last: 85,
    involvedObject: 250,
    actions: 50
};

// Average character width in pixels (approximate for our font)
export const CHAR_WIDTH_PX = 7.5;
// Padding for cells (px on each side)
export const CELL_PADDING_PX = 24;

// Column definition interface
export interface ColumnDef {
    key: string;
    label?: string | React.ReactNode;
    render?: (item: any) => React.ReactNode;
    getValue?: (item: any) => any;
    isColumnSelector?: boolean;
    isSelectionColumn?: boolean;
    defaultHidden?: boolean;
    filterable?: boolean;
    disableSort?: boolean;
    align?: 'left' | 'center' | 'right';
    initialSort?: 'asc' | 'desc';
}

// Column widths map type
export type ColumnWidths = Record<string, number>;

// Calculate width needed for a string value
export const calculateTextWidth = (text: unknown): number => {
    if (!text) return 0;
    const str = String(text);
    return Math.ceil(str.length * CHAR_WIDTH_PX) + CELL_PADDING_PX;
};

// Calculate column widths based on data - finds width that covers ~95% of values
export const calculateColumnWidths = (columns: ColumnDef[], data: any[], savedWidths: ColumnWidths): ColumnWidths => {
    if (!data || data.length === 0) return {};

    const calculatedWidths: ColumnWidths = {};

    // Columns that render visual components with fixed widths - skip auto-calculation
    const fixedWidthColumns = new Set(['cpu', 'memory', 'pods']);

    for (const col of columns) {
        // Skip columns that already have saved user widths
        if (savedWidths[col.key]) continue;
        // Skip special columns
        if (col.isColumnSelector || col.isSelectionColumn) continue;
        // Skip columns that render fixed-width visual components (e.g., resource bars)
        if (fixedWidthColumns.has(col.key)) continue;

        const minWidth = MIN_COLUMN_WIDTHS[col.key] || 80;

        // Calculate header width
        const headerWidth = calculateTextWidth(col.label) + 20; // extra for sort indicator

        // Sample data to find content widths
        const contentWidths: number[] = [];
        const sampleSize = Math.min(data.length, 500); // Sample up to 500 rows
        const step = Math.max(1, Math.floor(data.length / sampleSize));

        for (let i = 0; i < data.length; i += step) {
            const item = data[i];
            let value;

            // Get the display value
            if (col.getValue) {
                value = col.getValue(item);
            } else if (col.key === 'name') {
                value = item.metadata?.name;
            } else if (col.key === 'namespace') {
                value = item.metadata?.namespace;
            } else {
                value = item[col.key];
            }

            // For rendered content, try to extract text
            if (col.render) {
                // For rendered columns, use the raw value or a reasonable estimate
                if (typeof value === 'string') {
                    contentWidths.push(calculateTextWidth(value));
                } else if (col.key === 'name' && item.metadata?.name) {
                    contentWidths.push(calculateTextWidth(item.metadata.name));
                }
            } else if (value !== null && value !== undefined) {
                contentWidths.push(calculateTextWidth(value));
            }
        }

        if (contentWidths.length === 0) {
            calculatedWidths[col.key] = Math.max(minWidth, headerWidth);
            continue;
        }

        // Sort widths and find 95th percentile
        contentWidths.sort((a: number, b: number) => a - b);
        const p95Index = Math.floor(contentWidths.length * 0.95);
        const p95Width = contentWidths[p95Index] || contentWidths[contentWidths.length - 1];

        // Use the larger of: min width, header width, or 95th percentile content width
        // Cap at a reasonable max to prevent extremely wide columns
        const maxWidth = col.key === 'message' ? 500 : 350;
        calculatedWidths[col.key] = Math.min(maxWidth, Math.max(minWidth, headerWidth, p95Width));
    }

    return calculatedWidths;
};

// Format large numbers with locale separators
export const formatCount = (n: number) => n.toLocaleString();
