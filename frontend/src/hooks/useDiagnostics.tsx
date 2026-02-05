import { useState, useCallback } from 'react';
import { GetResourceDiff, GetDependencyGraph, GetFlowTimeline } from '../../wailsjs/go/main/App';

interface DiagnosticsError {
  message: string;
  code?: string;
}

interface DiffResult {
  diff: string;
  changes: {
    additions: number;
    deletions: number;
    modifications: number;
  };
}

interface DependencyNode {
  id: string;
  type: string;
  name: string;
  namespace?: string;
  [key: string]: any;
}

interface DependencyEdge {
  source: string;
  target: string;
  type: string;
  [key: string]: any;
}

interface DependencyGraph {
  nodes: DependencyNode[];
  edges: DependencyEdge[];
}

interface FlowTimelineEvent {
  timestamp: string;
  type: string;
  resource: string;
  message: string;
  [key: string]: any;
}

interface UseDiagnosticsReturn {
  loading: boolean;
  error: DiagnosticsError | null;
  getDiff: (
    sourceType: string,
    sourceName: string,
    sourceNamespace: string,
    sourceContext: string,
    targetType: string,
    targetName: string,
    targetNamespace: string,
    targetContext: string
  ) => Promise<DiffResult | null>;
  getDependencies: (
    resourceType: string,
    resourceName: string,
    namespace: string,
    context: string
  ) => Promise<DependencyGraph | null>;
  getFlowTimeline: (
    resourceType: string,
    resourceName: string,
    namespace: string,
    context: string
  ) => Promise<FlowTimelineEvent[] | null>;
  clearError: () => void;
}

/**
 * Hook for diagnostics operations (diff, dependencies, timeline)
 * @returns Object with loading state, error state, and diagnostic functions
 */
export const useDiagnostics = (): UseDiagnosticsReturn => {
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<DiagnosticsError | null>(null);

  const getDiff = useCallback(
    async (
      sourceType: string,
      sourceName: string,
      sourceNamespace: string,
      sourceContext: string,
      targetType: string,
      targetName: string,
      targetNamespace: string,
      targetContext: string
    ): Promise<DiffResult | null> => {
      setLoading(true);
      setError(null);
      try {
        const result = await GetResourceDiff(
          sourceType,
          sourceName,
          sourceNamespace,
          sourceContext,
          targetType,
          targetName,
          targetNamespace,
          targetContext
        );
        return result as DiffResult;
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to get resource diff';
        setError({ message: errorMessage });
        return null;
      } finally {
        setLoading(false);
      }
    },
    []
  );

  const getDependencies = useCallback(
    async (
      resourceType: string,
      resourceName: string,
      namespace: string,
      context: string
    ): Promise<DependencyGraph | null> => {
      setLoading(true);
      setError(null);
      try {
        const result = await GetDependencyGraph(resourceType, resourceName, namespace, context);
        return result as DependencyGraph;
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to get dependency graph';
        setError({ message: errorMessage });
        return null;
      } finally {
        setLoading(false);
      }
    },
    []
  );

  const getFlowTimeline = useCallback(
    async (
      resourceType: string,
      resourceName: string,
      namespace: string,
      context: string
    ): Promise<FlowTimelineEvent[] | null> => {
      setLoading(true);
      setError(null);
      try {
        const result = await GetFlowTimeline(resourceType, resourceName, namespace, context);
        return result as FlowTimelineEvent[];
      } catch (err) {
        const errorMessage = err instanceof Error ? err.message : 'Failed to get flow timeline';
        setError({ message: errorMessage });
        return null;
      } finally {
        setLoading(false);
      }
    },
    []
  );

  const clearError = useCallback((): void => {
    setError(null);
  }, []);

  return {
    loading,
    error,
    getDiff,
    getDependencies,
    getFlowTimeline,
    clearError,
  };
};
