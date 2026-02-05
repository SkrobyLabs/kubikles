import { useState, useCallback } from 'react';

interface ResourceReference {
  type: string;
  name: string;
  namespace?: string;
  context?: string;
}

interface ComparisonState {
  source: ResourceReference | null;
  target: ResourceReference | null;
  isComparing: boolean;
}

interface UseComparisonReturn extends ComparisonState {
  setSource: (resource: ResourceReference | null) => void;
  setTarget: (resource: ResourceReference | null) => void;
  startComparison: () => void;
  clearComparison: () => void;
  swapResources: () => void;
}

/**
 * Hook for managing resource comparison state
 * @returns Object with comparison state and control functions
 */
export const useComparison = (): UseComparisonReturn => {
  const [state, setState] = useState<ComparisonState>({
    source: null,
    target: null,
    isComparing: false,
  });

  const setSource = useCallback((resource: ResourceReference | null): void => {
    setState((prev) => ({ ...prev, source: resource }));
  }, []);

  const setTarget = useCallback((resource: ResourceReference | null): void => {
    setState((prev) => ({ ...prev, target: resource }));
  }, []);

  const startComparison = useCallback((): void => {
    setState((prev) => ({ ...prev, isComparing: true }));
  }, []);

  const clearComparison = useCallback((): void => {
    setState({
      source: null,
      target: null,
      isComparing: false,
    });
  }, []);

  const swapResources = useCallback((): void => {
    setState((prev) => ({
      ...prev,
      source: prev.target,
      target: prev.source,
    }));
  }, []);

  return {
    ...state,
    setSource,
    setTarget,
    startComparison,
    clearComparison,
    swapResources,
  };
};
