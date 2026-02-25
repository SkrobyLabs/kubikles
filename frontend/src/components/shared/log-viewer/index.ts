/**
 * LogViewer module - A feature-rich log viewer for Kubernetes pods.
 *
 * Features:
 * - Real-time log streaming with auto-follow
 * - Chunk-based pagination (load older/newer logs)
 * - Search with regex support and match highlighting
 * - Filter mode with context lines
 * - ANSI color code support
 * - Multi-pod/container selection
 * - Download logs (single container, all pods, or visible)
 * - Time-based navigation
 *
 * Usage:
 *   import LogViewer from '../components/shared/log-viewer';
 *   // or
 *   import { LogViewer } from '../components/shared/log-viewer';
 */

export { default } from './LogViewer';
export { default as LogViewer } from './LogViewer';
export { default as DeferredLogViewer } from './DeferredLogViewer';
export type { ResolvedLogViewerProps } from './DeferredLogViewer';

// Export sub-components for advanced usage
export { LogLine, Spinner } from './LogLine';
export { TimePickerModal } from './TimePickerModal';

// Export hooks for custom implementations
export { useLogStream, ALL_CONTAINERS, ALL_PODS } from './useLogStream';
export { useLogSearch } from './useLogSearch';

// Export utilities
export * from './logUtils';
