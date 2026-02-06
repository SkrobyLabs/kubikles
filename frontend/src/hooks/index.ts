// Hooks barrel - commonly used hooks
// For resource-specific hooks, import from ~/hooks/resources

// UI & interaction hooks
export { useMenuPosition } from './useMenuPosition';
export { useSelection } from './useSelection';
export { useBulkActions } from './useBulkActions';
export { useClipboard } from './useClipboard';
export { useDebounce } from './useDebounce';
export { useLocalStorage } from './useLocalStorage';

// Resource action hooks
export { useBaseResourceActions } from './useBaseResourceActions';

// Feature hooks
export { usePerformancePanel } from './usePerformancePanel';
export { usePortForwards } from './usePortForwards';
export { useSavedViews } from './useSavedViews';
export { useIngressForward } from './useIngressForward';

// Metrics hooks
export { usePodMetrics } from './usePodMetrics';
export { useNodeMetrics } from './useNodeMetrics';
export { useNamespaceMetrics } from './useNamespaceMetrics';
export { useClusterMetrics } from './useClusterMetrics';

// Re-export resource hooks
export * from './resources';
